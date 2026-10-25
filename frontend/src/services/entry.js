import { entryApi, revealApi, slipApi } from './api'
import { lockUntil } from './timelock'
import {
  COUPON_KEY_MESSAGE, claimIx, claimSlipIx, couponCommitment, couponCount, couponKeyFromSignature, couponPda, eventPda, fetchEvent,
  picksFromSystem, placeSlipIx, PROGRAM_ID, refundIx, refundSlipIx, revealCouponIx, revealSlipIx, scoreSlipIx, sealCoupon,
  sendProgramTransaction, systemFromPicks, unsealCoupon, vaultSchedule, walletCoupons
} from './program'
import { publishedSlipPatch } from './slipStatus'
import { hexToBytes, slipLeaf, slipMark, slipRevealLeaf, sparseProof } from './slipTree'
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
const hexBytes = bytes => [...bytes].map(byte => byte.toString(16).padStart(2, '0')).join('')

function leavesOf (entries) {
  const leaves = []
  const sorted = [...(entries || [])].sort((a, b) => a.index - b.index)
  for (let index = 0; index < sorted.length; index += 1) {
    const row = sorted[index]
    if (row.index !== index) throw new Error('The coupon list is still catching up. Nothing was paid; please submit again.')
    leaves.push(slipLeaf(row.index, row.owner, row.combinations, row.commitment))
  }
  return leaves
}

function markedNodes (rows, count) {
  const nodes = Array.from({ length: count }, () => null)
  for (const row of rows || []) nodes[row.index] = slipMark()
  return nodes
}

async function slipProofs (eventAddress, index) {
  const archive = await slipApi.get(eventAddress)
  const leaves = leavesOf(archive.entries)
  const reveals = Array.from({ length: leaves.length }, () => null)
  for (const row of archive.reveals || []) {
    reveals[row.index] = slipRevealLeaf(row.index, row.owner, row.combinations, row.commitment, hexToBytes(row.picks))
  }
  const taken = [...(archive.claims || []), ...(archive.refunds || [])]
  return { archive, leaves, reveals, claimed: markedNodes(taken, leaves.length) }
}

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
  const archive = await slipApi.get(eventAddress).catch(() => ({ entries: [] }))
  const leaves = leavesOf(archive.entries || [])
  if (leaves.length !== index) throw new Error('The coupon list is still catching up. Nothing was paid; please submit again.')
  let sent
  try {
    sent = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
      placeSlipIx({ player: wallet, admin, nonce, commitment, combinations, proof: sparseProof(leaves, index) })
    ])
  } catch (error) {
    if (/already in use/i.test(error.message)) {
      throw new Error('Another player took this coupon number at the same moment. Nothing was paid; please submit again.', { cause: error })
    }
    throw error
  }
  const { signature, lastValidBlockHeight } = sent
  const coupon = {
    signature, programId: PROGRAM_ID, eventId: event.id, eventTitle: event.title, admin, nonce, index, slip: true,
    address: `slip:${eventAddress}:${index}`, revealNonce: revealNonce.toString(), commitment: hexBytes(commitment),
    system, combinations, lamports: event.couponPriceLamports * combinations, placedAt: Date.now(), revealed: false
  }
  rememberCoupon(wallet, coupon)
  onStep('confirm')
  try {
    await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
    await slipApi.ingest(signature).catch(() => {})
  } catch (error) {
    if (/No SOL was sent|failed on chain/.test(error.message)) {
      saveChainCoupons(wallet, loadChainCoupons(wallet).filter(item => item.signature !== signature))
    }
    throw error
  }
  onStep('schedule')
  try {
    const { deadline, revealDeadline } = vaultSchedule(event)
    await armCouponReveal({ wallet, coupon, deadline, revealDeadline })
  } catch (error) {
    coupon.armError = error.message
  }
  window.dispatchEvent(new Event(COUPON_PLACED))
  return coupon
}

const armedKey = wallet => `sol.revealArmed.v2.${wallet}`

export function loadArmed (wallet) {
  try {
    return JSON.parse(localStorage.getItem(armedKey(wallet))) || []
  } catch {
    return []
  }
}

function rememberArmed (wallet, address) {
  const list = loadArmed(wallet).filter(item => item !== address)
  list.push(address)
  localStorage.setItem(armedKey(wallet), JSON.stringify(list.slice(-200)))
}

// Time-locks the committed picks until entries close. No second signature: after the deadline
// the keeper publishes them, and the program rejects anything that misses the commitment.
export async function armCouponReveal ({ wallet, coupon, deadline, revealDeadline }) {
  if (loadArmed(wallet).includes(coupon.address)) return
  if (Math.floor(Date.now() / 1000) >= revealDeadline) throw new Error('The reveal window has closed')
  if (!coupon.system || coupon.revealNonce == null) throw new Error('This browser does not have the picks for that coupon')
  const payload = new TextEncoder().encode(JSON.stringify({
    revealNonce: coupon.revealNonce.toString(),
    picks: [...picksFromSystem(coupon.system)]
  }))
  await revealApi.arm(coupon.address, await lockUntil(payload, deadline))
  rememberArmed(wallet, coupon.address)
}

export const COUPON_PLACED = 'sol:coupon-placed'
export const COUPONS_REVEALED = 'sol:coupons-revealed'
const REVEALS_PER_TRANSACTION = 6

// Backup for coupons the keeper has not published. The owner signs these in the browser.
// Returns how many were revealed, which coupons were due, and the next entry deadline.
export async function autoRevealCoupons ({ config, wallet, events, skip = new Set(), lagSeconds = 0 }) {
  const rpcUrl = config.solanaRpcUrl
  const unrevealed = (await walletCoupons(rpcUrl, wallet)).filter(coupon => !coupon.revealed && !skip.has(coupon.address))
  const states = new Map()
  for (const address of new Set(unrevealed.map(coupon => coupon.event))) {
    states.set(address, await fetchEvent(rpcUrl, address))
  }
  const now = Math.floor(Date.now() / 1000)
  const open = unrevealed.filter(coupon => states.get(coupon.event).status === 'open')
  const due = open.filter(coupon => states.get(coupon.event).deadline + lagSeconds <= now && now < states.get(coupon.event).revealDeadline)
  const upcoming = open.map(coupon => states.get(coupon.event).deadline + lagSeconds).filter(deadline => deadline > now)
  const result = { revealed: 0, due: due.map(coupon => coupon.address), nextDeadline: upcoming.length ? Math.min(...upcoming) : null }
  if (await syncPublishedSlips(wallet)) window.dispatchEvent(new Event(COUPONS_REVEALED))
  for (const coupon of loadChainCoupons(wallet).filter(item => item.slip && !item.revealed && !skip.has(item.address))) {
    const state = await fetchEvent(rpcUrl, eventPda(coupon.admin, coupon.nonce))
    if (state.status === 'open' && state.deadline + lagSeconds <= now && now < state.revealDeadline) {
      await revealOnChain({ config, wallet, coupon })
      result.revealed += 1
    }
  }
  if (!due.length) return result

  try {
    const savedById = () => new Map(loadChainCoupons(wallet).map(saved => [couponId(saved), saved]))
    let saved = savedById()
    const missing = due.filter(coupon => !backupMatches(wallet, saved.get(coupon.address), coupon))
    if (missing.length && events) {
      await recoverFromChain({ wallet, onChain: missing, events })
      saved = savedById()
    }
    const ready = due.map(coupon => saved.get(coupon.address)).filter((local, i) => backupMatches(wallet, local, due[i]))
    for (let start = 0; start < ready.length; start += REVEALS_PER_TRANSACTION) {
      const batch = ready.slice(start, start + REVEALS_PER_TRANSACTION)
      const { signature, lastValidBlockHeight } = await sendProgramTransaction(rpcUrl, wallet, batch.map(local => revealCouponIx({
        player: wallet, admin: local.admin, nonce: local.nonce, index: local.index,
        picks: picksFromSystem(local.system), revealNonce: BigInt(local.revealNonce)
      })))
      await waitForConfirmation(rpcUrl, signature, lastValidBlockHeight)
      for (const local of batch) patchCoupon(wallet, local, { revealed: true })
      result.revealed += batch.length
      window.dispatchEvent(new Event(COUPONS_REVEALED))
    }
  } catch (error) {
    error.due = result.due
    throw error
  }
  return result
}

function patchCoupon (wallet, coupon, patch) {
  const id = couponId(coupon)
  saveChainCoupons(wallet, loadChainCoupons(wallet).map(saved => couponId(saved) === id ? { ...saved, ...patch } : saved))
}

// Copies keeper publishes into this browser so a slip is not offered for reveal again.
export async function syncPublishedSlips (wallet) {
  const pending = loadChainCoupons(wallet).filter(item => item.slip && !item.claimed && !item.refunded)
  const byEvent = new Map()
  for (const coupon of pending) {
    const event = eventPda(coupon.admin, coupon.nonce)
    if (!byEvent.has(event)) byEvent.set(event, [])
    byEvent.get(event).push(coupon)
  }
  let changed = false
  for (const [event, rows] of byEvent) {
    let archive
    try {
      archive = await slipApi.get(event)
    } catch {
      continue
    }
    for (const coupon of rows) {
      const patch = publishedSlipPatch(coupon, archive, wallet)
      if (!Object.keys(patch).length) continue
      patchCoupon(wallet, coupon, patch)
      changed = true
    }
  }
  return changed
}

export async function revealOnChain ({ config, wallet, coupon }) {
  const picks = picksFromSystem(coupon.system)
  const instructions = coupon.slip
    ? [await revealSlipInstruction(wallet, coupon, picks)]
    : [revealCouponIx({
      player: wallet, admin: coupon.admin, nonce: coupon.nonce, index: coupon.index,
      picks, revealNonce: BigInt(coupon.revealNonce)
    })]
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, instructions)
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  if (coupon.slip) await slipApi.ingest(signature).catch(() => {})
  patchCoupon(wallet, coupon, { revealed: true })
}

async function revealSlipInstruction (caller, coupon, picks) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { reveals } = await slipProofs(event, coupon.index)
  const commitment = hexToBytes(coupon.commitment)
  return revealSlipIx({
    caller, admin: coupon.admin, nonce: coupon.nonce, index: coupon.index, owner: caller,
    combinations: coupon.combinations, picks, revealNonce: BigInt(coupon.revealNonce), commitment,
    proof: sparseProof(reveals, coupon.index)
  })
}

export async function claimOnChain ({ config, wallet, coupon }) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const instructions = coupon.slip
    ? [await claimSlipInstruction(wallet, coupon)]
    : [claimIx({ owner: wallet, event, coupon: couponPda(event, coupon.index) })]
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, instructions)
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  if (coupon.slip) await slipApi.ingest(signature).catch(() => {})
  patchCoupon(wallet, coupon, { claimed: true })
}

async function claimSlipInstruction (owner, coupon) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { leaves, reveals, claimed } = await slipProofs(event, coupon.index)
  const picks = picksFromSystem(coupon.system)
  const commitment = hexToBytes(coupon.commitment)
  const reveal = {
    index: coupon.index, owner, combinations: coupon.combinations, picks,
    revealNonce: BigInt(coupon.revealNonce), commitment
  }
  reveals[coupon.index] = slipRevealLeaf(coupon.index, owner, coupon.combinations, commitment, picks)
  return claimSlipIx({
    owner, admin: coupon.admin, nonce: coupon.nonce, reveal,
    entryProof: sparseProof(leaves, coupon.index),
    revealProof: sparseProof(reveals, coupon.index),
    claimedProof: sparseProof(claimed, coupon.index)
  })
}

export async function refundOnChain ({ config, wallet, coupon }) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const instructions = coupon.slip
    ? [await refundSlipInstruction(wallet, coupon)]
    : [refundIx({ owner: wallet, event, coupon: couponPda(event, coupon.index) })]
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, instructions)
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  if (coupon.slip) await slipApi.ingest(signature).catch(() => {})
  patchCoupon(wallet, coupon, { refunded: true })
}

async function refundSlipInstruction (owner, coupon) {
  const event = eventPda(coupon.admin, coupon.nonce)
  const { leaves, claimed } = await slipProofs(event, coupon.index)
  return refundSlipIx({
    owner, admin: coupon.admin, nonce: coupon.nonce, index: coupon.index,
    combinations: coupon.combinations, commitment: hexToBytes(coupon.commitment),
    entryProof: sparseProof(leaves, coupon.index), refundProof: sparseProof(claimed, coupon.index)
  })
}

export async function scoreNextSlip ({ config, wallet, admin, event }) {
  const state = await fetchEvent(config.solanaRpcUrl, event)
  const index = state.tallied
  const { archive, leaves, reveals } = await slipProofs(event, index)
  const row = (archive.entries || []).find(entry => entry.index === index)
  if (!row) throw new Error('The coupon list is still catching up. Try the count again.')
  const revealedRow = (archive.reveals || []).find(entry => entry.index === index)
  const picks = revealedRow ? hexToBytes(revealedRow.picks) : new Uint8Array(13)
  const commitment = hexToBytes(row.commitment)
  if (revealedRow) reveals[index] = slipRevealLeaf(index, row.owner, row.combinations, commitment, picks)
  const scored = Array.from({ length: index }, () => slipMark())
  const reveal = {
    index, owner: row.owner, combinations: row.combinations, picks,
    revealNonce: 0n, commitment
  }
  const { signature, lastValidBlockHeight } = await sendProgramTransaction(config.solanaRpcUrl, wallet, [
    scoreSlipIx({
      caller: wallet, admin, event, reveal, revealed: !!revealedRow,
      entryProof: sparseProof(leaves, index),
      revealProof: sparseProof(reveals, index),
      scoredProof: sparseProof(scored, index)
    })
  ])
  await waitForConfirmation(config.solanaRpcUrl, signature, lastValidBlockHeight)
  await slipApi.ingest(signature).catch(() => {})
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
