import { entryApi } from './api'
import {
  couponCommitment, couponCount, couponPda, eventPda, picksFromSystem, placeCouponIx,
  PROGRAM_ID, revealCouponIx, claimIx, sendProgramTransaction
} from './program'
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

export async function paymentMemo (eventId, system) {
  const canonical = `${eventId}\n${system.join(',')}`
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(canonical))
  const hex = [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('')
  return `sol-entry-v1:${hex}`
}

// The RPC node used by the backend can lag a few seconds behind the one that confirmed the payment.
async function submitWithRetry (eventId, system, signature) {
  for (let attempt = 1; ; attempt++) {
    try {
      return await entryApi.create(eventId, system, signature)
    } catch (error) {
      if (attempt >= 5 || !/not found or not (?:confirmed|finalized)|rate limit/i.test(error.message)) throw error
      await sleep(2000 * attempt)
    }
  }
}

const chainKey = wallet => `sol.chainCoupons.${wallet}`

export function loadChainCoupons (wallet) {
  try {
    const saved = JSON.parse(localStorage.getItem(chainKey(wallet))) || []
    const current = saved.filter(coupon => coupon.programId === PROGRAM_ID)
    if (current.length !== saved.length) saveChainCoupons(wallet, current)
    return current
  } catch {
    return []
  }
}

function saveChainCoupons (wallet, list) {
  if (list.length) localStorage.setItem(chainKey(wallet), JSON.stringify(list))
  else localStorage.removeItem(chainKey(wallet))
}

export function updateChainCoupon (wallet, signature, patch) {
  saveChainCoupons(wallet, loadChainCoupons(wallet).map(coupon => coupon.signature === signature ? { ...coupon, ...patch } : coupon))
}

export async function placeOnChain ({ config, wallet, event, system, combinations, onStep = () => {} }) {
  onStep('sign')
  const admin = event.chain.admin
  const nonce = event.chain.nonce
  const eventAddress = eventPda(admin, nonce)
  const index = await couponCount(config.solanaRpcUrl, eventAddress)
  const picks = picksFromSystem(system)
  const revealNonce = crypto.getRandomValues(new BigUint64Array(1))[0]
  const commitment = couponCommitment(wallet, picks, revealNonce)
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    placeCouponIx({ player: wallet, admin, nonce, index, commitment, combinations })
  ])
  const coupon = {
    signature, programId: PROGRAM_ID, eventId: event.id, eventTitle: event.title, admin, nonce, index,
    revealNonce: revealNonce.toString(), system, combinations,
    lamports: event.couponPriceLamports * combinations, placedAt: Date.now(), revealed: false
  }
  saveChainCoupons(wallet, [...loadChainCoupons(wallet), coupon])
  onStep('confirm')
  try {
    await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  } catch (error) {
    if (/No SOL was sent|failed on chain/.test(error.message)) {
      saveChainCoupons(wallet, loadChainCoupons(wallet).filter(item => item.signature !== signature))
    }
    throw error
  }
  return coupon
}

export async function revealOnChain ({ config, wallet, coupon }) {
  const picks = picksFromSystem(coupon.system)
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    revealCouponIx({
      player: wallet, admin: coupon.admin, nonce: coupon.nonce, index: coupon.index,
      picks, revealNonce: BigInt(coupon.revealNonce)
    })
  ])
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  updateChainCoupon(wallet, coupon.signature, { revealed: true })
}

export async function claimOnChain ({ config, wallet, coupon }) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    claimIx({ owner: wallet, event, coupon: couponPda(event, coupon.index) })
  ])
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  updateChainCoupon(wallet, coupon.signature, { claimed: true })
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
  const memo = await paymentMemo(event.id, system)
  const { signature, lastValidBlockHeight } = await sendPayment(config.solanaRpcUrl, wallet, config.treasuryWallet, lamports, memo)
  const pending = { signature, eventId: event.id, eventTitle: event.title, system, combinations, lamports, memo, paidAt: Date.now() }
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
