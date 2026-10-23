import { sha256 } from '@noble/hashes/sha256'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Decode, base58Encode, getAccount, getProgramAccounts, latestBlockhash, signAndSendMessage } from './solana.js'

export const PROGRAM_ID = 'H4x1HKyzxCgLEJDWHpSERvr4YveennDDPH3VdjkG97tr'
export const SYSTEM_PROGRAM = '11111111111111111111111111111111'
export const COMPUTE_BUDGET_PROGRAM = 'ComputeBudget111111111111111111111111111111'
export const RESULT_COSIGNER = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
export const COUPON_COUNT_OFFSET = 130
export const MAX_OPEN_VAULT_EVENTS = 16
export const VOID_RESULT = 'C'
// Coupons per tally transaction. Each one adds 33 bytes to a 1232-byte transaction.
export const TALLY_BATCH = 12
export const RESOLVE_WINDOW_SECONDS = 7 * 24 * 3600

const EVENT_ACCOUNT_SIZE = 8 + 448
const COUPON_ACCOUNT_SIZE = 8 + 149
const SEALED_LEN = 21
export const EVENT_STATUSES = ['open', 'resolved', 'cancelled', 'tallying']

const text = value => new TextEncoder().encode(value)

function concat (parts) {
  const out = new Uint8Array(parts.reduce((total, part) => total + part.length, 0))
  let offset = 0
  for (const part of parts) {
    out.set(part, offset)
    offset += part.length
  }
  return out
}

function u32 (value) {
  const out = new Uint8Array(4)
  new DataView(out.buffer).setUint32(0, value, true)
  return out
}

function u64 (value) {
  const out = new Uint8Array(8)
  new DataView(out.buffer).setBigUint64(0, BigInt(value), true)
  return out
}

const compactU16 = value => {
  const out = []
  let rest = value
  for (;;) {
    const byte = rest & 0x7f
    rest >>= 7
    if (rest === 0) return Uint8Array.from([...out, byte])
    out.push(byte | 0x80)
  }
}

function onCurve (bytes) {
  try {
    ed25519.ExtendedPoint.fromHex(bytes)
    return true
  } catch {
    return false
  }
}

export function findPda (seeds) {
  const program = base58Decode(PROGRAM_ID, 32)
  for (let bump = 255; bump >= 0; bump--) {
    const hash = sha256(concat([...seeds, Uint8Array.of(bump), program, text('ProgramDerivedAddress')]))
    if (!onCurve(hash)) return base58Encode(hash)
  }
  throw new Error('No program address for those seeds')
}

export function eventPda (admin, nonce) {
  return findPda([text('event'), base58Decode(admin, 32), u64(nonce)])
}

export function vaultPda (event) {
  return findPda([text('vault'), base58Decode(event, 32)])
}

export function couponPda (event, index) {
  return findPda([text('coupon'), base58Decode(event, 32), u32(index)])
}

export function gatePda (admin) {
  return findPda([text('gate'), base58Decode(admin, 32)])
}

export function schedulePda (admin) {
  return findPda([text('schedule'), base58Decode(admin, 32)])
}

export function signersPda (event) {
  return findPda([text('signers'), base58Decode(event, 32)])
}

export function escrowPda (admin) {
  return findPda([text('rollover-escrow'), base58Decode(admin, 32)])
}

export function couponCommitment (player, picks, nonce) {
  return sha256(concat([base58Decode(player, 32), picks, u64(nonce)]))
}

export function picksFromSystem (system) {
  const bits = { 1: 0b001, X: 0b010, 2: 0b100 }
  const picks = new Uint8Array(13)
  system.forEach((game, index) => {
    picks[index] = [...game].reduce((mask, outcome) => mask | bits[outcome], 0)
  })
  return picks
}

function discriminator (name) {
  return sha256(text(`global:${name}`)).slice(0, 8)
}

function ix (name, accounts, data) {
  return { programId: PROGRAM_ID, accounts, data: concat([discriminator(name), data]) }
}

const writable = pubkey => ({ pubkey, isSigner: false, isWritable: true })
const readonly = pubkey => ({ pubkey, isSigner: false, isWritable: false })
const signer = (pubkey, isWritable) => ({ pubkey, isSigner: true, isWritable })

// Reveals stay open until the end time, which is also the earliest resolution time.
// The backend checks these exact values before it attaches a vault to an event.
export function vaultSchedule (event) {
  const deadline = Math.floor(new Date(event.startTime).getTime() / 1000)
  const endTime = Math.floor(new Date(event.endTime).getTime() / 1000)
  if (!(endTime > deadline)) throw new Error('The event end time must be after entries close')
  return { deadline, revealDeadline: endTime, endTime, resolveTimeout: endTime + RESOLVE_WINDOW_SECONDS }
}

export function createEventIx ({ admin, other, nonce, deadline, revealDeadline, endTime, resolveTimeout, gameCount = 7, price = 1_000_000, tiers }) {
  const event = eventPda(admin, nonce)
  const prizeTiers = tiers?.length ? tiers : [{ correct: gameCount, percent: 95 }]
  const data = concat([
    u64(nonce), Uint8Array.of(gameCount), u64(price), u64(deadline), u64(revealDeadline), u64(endTime ?? revealDeadline), u64(resolveTimeout ?? revealDeadline + 10_000),
    Uint8Array.of(2), u32(2), base58Decode(admin, 32), base58Decode(other, 32),
    u32(prizeTiers.length), Uint8Array.from(prizeTiers.flatMap(tier => [tier.correct, tier.percent]))
  ])
  return ix('create_event', [
    signer(admin, true), writable(event), writable(vaultPda(event)), writable(escrowPda(admin)),
    writable(schedulePda(admin)), writable(signersPda(event)), writable(gatePda(admin)), readonly(SYSTEM_PROGRAM)
  ], data)
}

export function placeCouponIx ({ player, admin, nonce, index, commitment, combinations, sealed = new Uint8Array(SEALED_LEN) }) {
  const event = eventPda(admin, nonce)
  if (sealed.length !== SEALED_LEN) throw new Error('The sealed coupon backup has the wrong size')
  return ix('place_coupon', [
    signer(player, true), writable(event), writable(vaultPda(event)), writable(couponPda(event, index)),
    readonly(gatePda(admin)), readonly(SYSTEM_PROGRAM)
  ], concat([commitment, u32(combinations), sealed]))
}

export function revealCouponIx ({ player, admin, nonce, index, picks, revealNonce }) {
  const event = eventPda(admin, nonce)
  return ix('reveal_coupon', [
    signer(player, false), writable(couponPda(event, index)), readonly(event)
  ], concat([picks, u64(revealNonce)]))
}

const approveAccounts = (signerKey, admin, event) => [
  signer(signerKey, false), writable(event), readonly(signersPda(event)), writable(schedulePda(admin))
]

// results is a string such as "1X21C11": one of 1, X, 2 or C (void) per game.
export function approveResolutionIx ({ signer: signerKey, admin, event, results }) {
  if (!/^[1X2C]+$/.test(results)) throw new Error('Every game needs a result of 1, X, 2 or C')
  const bytes = text(results)
  return ix('approve_resolution', approveAccounts(signerKey, admin, event), concat([u32(bytes.length), bytes]))
}

export function approveCancelIx ({ signer: signerKey, admin, event }) {
  return ix('approve_cancel', approveAccounts(signerKey, admin, event), new Uint8Array())
}

export function tallyIx ({ admin, event, coupons }) {
  return ix('tally', [
    writable(event), readonly(vaultPda(event)), writable(schedulePda(admin)), ...coupons.map(readonly)
  ], new Uint8Array())
}

export function computeBudgetIx (units) {
  return { programId: COMPUTE_BUDGET_PROGRAM, accounts: [], data: concat([Uint8Array.of(2), u32(units)]) }
}

export function expireEventIx ({ caller, admin, event }) {
  return ix('expire_event', [signer(caller, false), writable(event), writable(schedulePda(admin))], new Uint8Array())
}

export function setPauseIx ({ admin, paused }) {
  return ix('set_pause', [signer(admin, false), writable(gatePda(admin))], Uint8Array.of(paused ? 1 : 0))
}

export function refundIx ({ owner, event, coupon }) {
  return ix('refund', [writable(coupon), writable(event), writable(vaultPda(event)), signer(owner, true)], new Uint8Array())
}

export function withdrawHouseIx ({ admin, event }) {
  return ix('withdraw_house', [signer(admin, true), writable(event), writable(vaultPda(event))], new Uint8Array())
}

export function forwardRolloverIx ({ admin, source, destination }) {
  return ix('forward_rollover', [
    writable(source), writable(vaultPda(source)), writable(destination), writable(vaultPda(destination)), writable(schedulePda(admin))
  ], new Uint8Array())
}

export function parkRolloverIx ({ admin, source }) {
  return ix('park_rollover', [
    writable(source), writable(vaultPda(source)), writable(escrowPda(admin)), writable(schedulePda(admin))
  ], new Uint8Array())
}

const view = data => new DataView(data.buffer, data.byteOffset, data.byteLength)
const accountDiscriminator = name => sha256(text(`account:${name}`)).slice(0, 8)
const sameBytes = (a, b) => a.length === b.length && a.every((byte, index) => byte === b[index])
const ascii = bytes => String.fromCharCode(...bytes)

export function decodeEvent (data) {
  if (!data || data.length < EVENT_ACCOUNT_SIZE || !sameBytes(data.slice(0, 8), accountDiscriminator('Event'))) {
    throw new Error('The account is not a vault event')
  }
  const v = view(data)
  const gameCount = data[48]
  const tierCount = data[57]
  const tiers = []
  for (let i = 0; i < tierCount; i++) tiers.push({ correct: data[58 + 2 * i], percent: data[59 + 2 * i] })
  const perWinner = []
  for (let i = 0; i < 13; i++) perWinner.push(v.getBigUint64(175 + 8 * i, true))
  const winners = []
  for (let i = 0; i < 14; i++) winners.push(v.getBigUint64(309 + 8 * i, true))
  return {
    admin: base58Encode(data.slice(8, 40)),
    nonce: v.getBigUint64(40, true),
    gameCount,
    price: v.getBigUint64(49, true),
    tiers,
    deadline: Number(v.getBigInt64(84, true)),
    revealDeadline: Number(v.getBigInt64(92, true)),
    endTime: Number(v.getBigInt64(100, true)),
    resolveTimeout: Number(v.getBigInt64(108, true)),
    status: EVENT_STATUSES[data[116]] ?? 'unknown',
    results: ascii(data.slice(117, 117 + gameCount)),
    couponCount: v.getUint32(130, true),
    sales: v.getBigUint64(134, true),
    rolloverIn: v.getBigUint64(142, true),
    house: v.getBigUint64(150, true),
    paid: v.getBigUint64(158, true),
    rolloverOut: v.getBigUint64(166, true),
    rolloverForwarded: data[174] === 1,
    // perWinner[n - 1] is the payout per row with n correct picks.
    perWinner,
    rentReserve: v.getBigUint64(281, true),
    houseWithdrawn: data[289] === 1,
    proposalKind: ['none', 'resolve', 'cancel'][data[290]] ?? 'none',
    proposalResults: ascii(data.slice(291, 291 + gameCount)),
    approvals: data[304],
    tallied: v.getUint32(305, true),
    winners
  }
}

export function decodeCoupon (data) {
  if (!data || data.length < COUPON_ACCOUNT_SIZE || !sameBytes(data.slice(0, 8), accountDiscriminator('Coupon'))) {
    throw new Error('The account is not a coupon')
  }
  const v = view(data)
  return {
    event: base58Encode(data.slice(8, 40)),
    owner: base58Encode(data.slice(40, 72)),
    index: v.getUint32(72, true),
    gameCount: data[76],
    picks: data.slice(77, 90),
    commitment: data.slice(90, 122),
    revealed: data[122] === 1,
    combinations: v.getUint32(123, true),
    lamportsPaid: v.getBigUint64(127, true),
    sealed: data.slice(136, 136 + SEALED_LEN)
  }
}

export function decodeSigners (data) {
  if (!data || data.length < 8 + 195 || !sameBytes(data.slice(0, 8), accountDiscriminator('ResultSigners'))) {
    throw new Error('The account is not a result signer list')
  }
  const count = data[41]
  const members = []
  for (let i = 0; i < count; i++) members.push(base58Encode(data.slice(42 + 32 * i, 74 + 32 * i)))
  return { event: base58Encode(data.slice(8, 40)), threshold: data[40], members }
}

export function decodeSchedule (data) {
  if (!data || data.length < 8 + 33 || !sameBytes(data.slice(0, 8), accountDiscriminator('Schedule'))) return { entries: [] }
  const v = view(data)
  const entries = []
  for (let i = 0; i < data[40]; i++) {
    const at = 41 + 40 * i
    entries.push({ event: base58Encode(data.slice(at, at + 32)), deadline: Number(v.getBigInt64(at + 32, true)) })
  }
  return { entries }
}

export async function fetchEvent (rpcUrl, address) {
  const data = await getAccount(rpcUrl, address)
  if (!data) throw new Error('The on-chain event was not found')
  return decodeEvent(data)
}

export async function fetchSchedule (rpcUrl, admin) {
  return decodeSchedule(await getAccount(rpcUrl, schedulePda(admin)))
}

export async function fetchGatePaused (rpcUrl, admin) {
  const data = await getAccount(rpcUrl, gatePda(admin))
  return Boolean(data && data.length > 40 && data[40] === 1)
}

export async function fetchSigners (rpcUrl, event) {
  return decodeSigners(await getAccount(rpcUrl, signersPda(event)))
}

// Every coupon account the wallet owns, straight from the program, so a cleared browser
// or a new device still finds them. Claimed or refunded coupons are closed and do not appear.
export async function walletCoupons (rpcUrl, owner) {
  const accounts = await getProgramAccounts(rpcUrl, PROGRAM_ID, [
    { dataSize: COUPON_ACCOUNT_SIZE },
    { memcmp: { offset: 40, bytes: owner } }
  ])
  return accounts.map(({ pubkey, data }) => ({ address: pubkey, ...decodeCoupon(data) }))
}

export async function eventCoupons (rpcUrl, event) {
  const accounts = await getProgramAccounts(rpcUrl, PROGRAM_ID, [
    { dataSize: COUPON_ACCOUNT_SIZE },
    { memcmp: { offset: 8, bytes: event } }
  ])
  return accounts.map(({ pubkey, data }) => ({ address: pubkey, ...decodeCoupon(data) })).sort((a, b) => a.index - b.index)
}

export async function couponCount (rpcUrl, eventAddress) {
  const data = await getAccount(rpcUrl, eventAddress)
  if (!data || data.length < COUPON_COUNT_OFFSET + 4) throw new Error('The on-chain event was not found')
  return new DataView(data.buffer, data.byteOffset, data.byteLength).getUint32(COUPON_COUNT_OFFSET, true)
}

export function systemFromPicks (picks, gameCount) {
  return [...picks.slice(0, gameCount)].map(mask => ['1', 'X', '2'].filter((_, bit) => mask & (1 << bit)).join(''))
}

// Rows of a system entry by number of correct picks; a void result (C) matches every pick.
export function correctCounts (picks, results) {
  let counts = [1]
  for (let game = 0; game < results.length; game++) {
    const result = results[game]
    const resultBits = result === VOID_RESULT ? 0b111 : { 1: 0b001, X: 0b010, 2: 0b100 }[result]
    const mask = picks[game] & 0b111
    const choices = popcount(mask)
    const correct = popcount(mask & resultBits)
    const next = new Array(counts.length + 1).fill(0)
    counts.forEach((rows, score) => {
      next[score] += rows * (choices - correct)
      next[score + 1] += rows * correct
    })
    counts = next
  }
  return counts
}

const popcount = mask => (mask & 1) + ((mask >> 1) & 1) + ((mask >> 2) & 1)

export function couponPayout (coupon, event) {
  if (!coupon.revealed || event.status !== 'resolved') return 0n
  const counts = correctCounts(coupon.picks, event.results)
  return event.tiers.reduce((sum, tier) => sum + event.perWinner[tier.correct - 1] * BigInt(counts[tier.correct] ?? 0), 0n)
}

// The sealed backup is the 13 pick masks and the reveal nonce, XORed with a keystream from
// a wallet-signature key and the coupon commitment. Only the wallet owner can open it.
export const COUPON_KEY_MESSAGE = wallet =>
  `Sol Predictions coupon key v1\nWallet: ${wallet}\nSigning this lets the app back up and recover your coupon picks. It does not move funds.`

export function couponKeyFromSignature (signature) {
  return sha256(signature)
}

function keystream (key, commitment) {
  return sha256(concat([key, commitment])).slice(0, SEALED_LEN)
}

export function sealCoupon (key, commitment, picks, revealNonce) {
  const plain = concat([picks, u64(revealNonce)])
  const stream = keystream(key, commitment)
  return plain.map((byte, index) => byte ^ stream[index])
}

export function unsealCoupon (key, coupon) {
  const stream = keystream(key, coupon.commitment)
  const plain = coupon.sealed.map((byte, index) => byte ^ stream[index])
  const picks = plain.slice(0, 13)
  const revealNonce = view(plain).getBigUint64(13, true)
  if (!sameBytes(couponCommitment(coupon.owner, picks, revealNonce), coupon.commitment)) return null
  return { picks, revealNonce }
}

export async function sendProgramTransaction (rpcUrl, feePayer, instructions) {
  const { value } = await latestBlockhash(rpcUrl)
  const { message } = compileMessage(feePayer, instructions, value.blockhash)
  if (message.length + 65 > 1232) throw new Error('The transaction is too large; use a smaller batch')
  const signature = await signAndSendMessage(message)
  return { signature, lastValidBlockHeight: value.lastValidBlockHeight }
}

export function claimIx ({ owner, event, coupon }) {
  return ix('claim', [
    writable(coupon), writable(event), writable(vaultPda(event)), signer(owner, true)
  ], new Uint8Array())
}

// Legacy transaction message. The fee payer is account 0 and stays writable.
export function compileMessage (feePayer, instructions, blockhash) {
  const metas = new Map()
  const touch = (pubkey, isSigner, isWritable) => {
    const current = metas.get(pubkey) || { isSigner: false, isWritable: false }
    current.isSigner = current.isSigner || isSigner
    current.isWritable = current.isWritable || isWritable
    metas.set(pubkey, current)
  }
  touch(feePayer, true, true)
  for (const instruction of instructions) {
    touch(instruction.programId, false, false)
    for (const account of instruction.accounts) touch(account.pubkey, account.isSigner, account.isWritable)
  }
  const groups = [[], [], [], []]
  for (const [pubkey, meta] of metas) {
    if (pubkey === feePayer) continue
    groups[(meta.isSigner ? 0 : 2) + (meta.isWritable ? 0 : 1)].push(pubkey)
  }
  const keys = [feePayer, ...groups.flat()]
  const at = pubkey => {
    const index = keys.indexOf(pubkey)
    if (index < 0) throw new Error(`Missing account ${pubkey}`)
    return index
  }
  const compiled = instructions.map(instruction => Uint8Array.from([
    at(instruction.programId),
    ...compactU16(instruction.accounts.length),
    ...instruction.accounts.map(account => at(account.pubkey)),
    ...compactU16(instruction.data.length),
    ...instruction.data
  ]))
  const signers = [feePayer, ...groups[0], ...groups[1]]
  return {
    signers,
    message: concat([
      Uint8Array.of(signers.length, groups[1].length, groups[3].length),
      compactU16(keys.length),
      ...keys.map(key => base58Decode(key, 32)),
      base58Decode(blockhash, 32),
      compactU16(compiled.length),
      ...compiled
    ])
  }
}
