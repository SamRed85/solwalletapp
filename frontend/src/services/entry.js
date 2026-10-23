import { entryApi } from './api'
import {
  COUPON_KEY_MESSAGE, couponCommitment, couponCount, couponKeyFromSignature, couponPda, eventPda, picksFromSystem,
  placeCouponIx, PROGRAM_ID, refundIx, revealCouponIx, claimIx, sealCoupon, sendProgramTransaction, systemFromPicks,
  unsealCoupon
} from './program'
import { sendPayment, signMessageBytes, waitForConfirmation } from './solana'

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

// Coupons are identified by their on-chain address; the entry signature is only known for
// coupons placed in this browser.
export const couponId = coupon => coupon.address || couponPda(eventPda(coupon.admin, coupon.nonce), coupon.index)

function rememberCoupon (wallet, coupon) {
  const id = couponId(coupon)
  saveChainCoupons(wallet, [...loadChainCoupons(wallet).filter(saved => couponId(saved) !== id), coupon])
}

const keyCacheName = wallet => `sol.couponKey.${wallet}`

// The key is derived from a deterministic wallet signature, so it can be recreated on any
// device. It is kept for this browser session only.
export async function couponKey (wallet) {
  const cached = sessionStorage.getItem(keyCacheName(wallet))
  if (cached) return Uint8Array.from(atob(cached), char => char.charCodeAt(0))
  const key = couponKeyFromSignature(await signMessageBytes(COUPON_KEY_MESSAGE(wallet)))
  sessionStorage.setItem(keyCacheName(wallet), btoa(String.fromCharCode(...key)))
  return key
}

export async function placeOnChain ({ config, wallet, event, system, combinations, onStep = () => {} }) {
  onStep('sign')
  const admin = event.chain.admin
  const nonce = event.chain.nonce
  const eventAddress = eventPda(admin, nonce)
  const key = await couponKey(wallet)
  const index = await couponCount(config.solanaRpcUrl, eventAddress)
  const picks = picksFromSystem(system)
  const revealNonce = crypto.getRandomValues(new BigUint64Array(1))[0]
  const commitment = couponCommitment(wallet, picks, revealNonce)
  const sealed = sealCoupon(key, commitment, picks, revealNonce)
  if (!unsealCoupon(key, { owner: wallet, commitment, sealed })) throw new Error('The coupon backup could not be verified')
  let sent
  try {
    sent = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
      placeCouponIx({ player: wallet, admin, nonce, index, commitment, combinations, sealed })
    ])
  } catch (error) {
    if (/already in use/i.test(error.message)) {
      throw new Error('Another player took this coupon number at the same moment. Nothing was paid; please submit again.', { cause: error })
    }
    throw error
  }
  const { signature, lastValidBlockHeight } = sent
  const coupon = {
    signature, programId: PROGRAM_ID, eventId: event.id, eventTitle: event.title, admin, nonce, index,
    address: couponPda(eventAddress, index), revealNonce: revealNonce.toString(), system, combinations,
    lamports: event.couponPriceLamports * combinations, placedAt: Date.now(), revealed: false
  }
  rememberCoupon(wallet, coupon)
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

function patchCoupon (wallet, coupon, patch) {
  const id = couponId(coupon)
  saveChainCoupons(wallet, loadChainCoupons(wallet).map(saved => couponId(saved) === id ? { ...saved, ...patch } : saved))
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
  patchCoupon(wallet, coupon, { revealed: true })
}

export async function claimOnChain ({ config, wallet, coupon }) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    claimIx({ owner: wallet, event, coupon: couponPda(event, coupon.index) })
  ])
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  patchCoupon(wallet, coupon, { claimed: true })
}

export async function refundOnChain ({ config, wallet, coupon }) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    refundIx({ owner: wallet, event, coupon: couponPda(event, coupon.index) })
  ])
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  patchCoupon(wallet, coupon, { refunded: true })
}

// Opens the sealed backups of on-chain coupons that this browser does not know, using the
// wallet-signature key. Returns how many coupons were restored.
export async function recoverFromChain ({ wallet, onChain, events }) {
  const known = new Set(loadChainCoupons(wallet).filter(saved => saved.revealNonce).map(couponId))
  const missing = onChain.filter(coupon => !coupon.revealed && !known.has(coupon.address))
  if (!missing.length) return 0
  const key = await couponKey(wallet)
  let restored = 0
  for (const coupon of missing) {
    const opened = unsealCoupon(key, coupon)
    const event = events.get(coupon.event)
    if (!opened || !event) continue
    rememberCoupon(wallet, {
      programId: PROGRAM_ID, eventId: event.id, eventTitle: event.title, admin: event.chain.admin, nonce: event.chain.nonce,
      index: coupon.index, address: coupon.address, revealNonce: opened.revealNonce.toString(),
      system: systemFromPicks(opened.picks, coupon.gameCount), combinations: coupon.combinations,
      lamports: Number(coupon.lamportsPaid), placedAt: Date.now(), revealed: false
    })
    restored++
  }
  if (restored < missing.length) {
    throw new Error(`Restored ${restored} of ${missing.length} coupons. The rest need a backup file from the browser that placed them.`)
  }
  return restored
}

export function exportCouponBackup (wallet) {
  return JSON.stringify({ version: 1, wallet, programId: PROGRAM_ID, coupons: loadChainCoupons(wallet) }, null, 2)
}

export function importCouponBackup (wallet, textValue) {
  const backup = JSON.parse(textValue)
  if (backup?.version !== 1 || backup.wallet !== wallet || backup.programId !== PROGRAM_ID || !Array.isArray(backup.coupons)) {
    throw new Error('This backup belongs to a different wallet or program')
  }
  let added = 0
  for (const coupon of backup.coupons) {
    if (!coupon.revealNonce || !Array.isArray(coupon.system) || coupon.programId !== PROGRAM_ID) continue
    rememberCoupon(wallet, { ...coupon, address: couponId(coupon) })
    added++
  }
  return added
}

// True when the saved picks and nonce open the on-chain commitment, so a reveal will succeed.
export function backupMatches (wallet, saved, onChain) {
  if (!saved?.revealNonce || !saved.system) return false
  const commitment = couponCommitment(wallet, picksFromSystem(saved.system), BigInt(saved.revealNonce))
  return commitment.every((byte, index) => byte === onChain.commitment[index])
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
