import { entryApi } from './api'
import { sendPayment, waitForConfirmation } from './solana'

// Payments are kept here until the backend has accepted the entry, so a closed tab or a
// failed request does not lose a paid coupon. The backend accepts payments up to 1 hour old.
export const PAYMENT_MAX_AGE_MS = 60 * 60 * 1000

const key = wallet => `sol.pendingPayments.${wallet}`

export function loadPending (wallet) {
  try {
    return JSON.parse(localStorage.getItem(key(wallet))) || []
  } catch {
    return []
  }
}

function savePending (wallet, list) {
  if (list.length) localStorage.setItem(key(wallet), JSON.stringify(list))
  else localStorage.removeItem(key(wallet))
}

export function removePending (wallet, signature) {
  savePending(wallet, loadPending(wallet).filter(p => p.signature !== signature))
}

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

// The RPC node used by the backend can lag a few seconds behind the one that confirmed the payment.
async function submitWithRetry (eventId, system, signature) {
  for (let attempt = 1; ; attempt++) {
    try {
      return await entryApi.create(eventId, system, signature)
    } catch (error) {
      if (attempt >= 5 || !/not found or not confirmed|rate limit/i.test(error.message)) throw error
      await sleep(2000 * attempt)
    }
  }
}

export async function submitPending (wallet, pending) {
  const entry = await submitWithRetry(pending.eventId, pending.system, pending.signature)
  removePending(wallet, pending.signature)
  return entry
}

// onStep receives 'sign', 'confirm' and 'submit' so the UI can show progress.
export async function payAndEnter ({ config, wallet, event, system, combinations, onStep = () => {} }) {
  onStep('sign')
  const lamports = event.couponPriceLamports * combinations
  const { signature, lastValidBlockHeight } = await sendPayment(config.solanaRpcUrl, wallet, config.treasuryWallet, lamports)
  const pending = { signature, eventId: event.id, eventTitle: event.title, system, combinations, lamports, paidAt: Date.now() }
  savePending(wallet, [...loadPending(wallet), pending])

  onStep('confirm')
  try {
    await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  } catch (error) {
    if (/No SOL was sent|failed on chain/.test(error.message)) removePending(wallet, signature)
    throw error
  }

  onStep('submit')
  return submitPending(wallet, pending)
}
