import { sha256 } from '@noble/hashes/sha256'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Decode, base58Encode, getAccount, latestBlockhash, signAndSendMessage } from './solana.js'

export const PROGRAM_ID = '35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca'
export const SYSTEM_PROGRAM = '11111111111111111111111111111111'
export const RESULT_COSIGNER = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
export const COUPON_COUNT_OFFSET = 130

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

export function escrowPda () {
  return findPda([text('rollover-escrow')])
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

export function vaultSchedule (event) {
  const deadline = Math.floor(new Date(event.startTime).getTime() / 1000)
  const endTime = Math.floor(new Date(event.endTime).getTime() / 1000)
  const revealDeadline = Math.min(endTime, deadline + 3600)
  if (!(revealDeadline > deadline && endTime >= revealDeadline)) {
    throw new Error('The event needs time after entries close for picks to be revealed')
  }
  return { deadline, revealDeadline, endTime, resolveTimeout: endTime + 7 * 24 * 3600 }
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
    signer(admin, true), writable(event), writable(vaultPda(event)), writable(escrowPda()),
    writable(schedulePda(admin)), writable(signersPda(event)), writable(gatePda(admin)), readonly(SYSTEM_PROGRAM)
  ], data)
}

export function placeCouponIx ({ player, admin, nonce, index, commitment, combinations }) {
  const event = eventPda(admin, nonce)
  return ix('place_coupon', [
    signer(player, true), writable(event), writable(vaultPda(event)), writable(couponPda(event, index)),
    readonly(gatePda(admin)), readonly(SYSTEM_PROGRAM)
  ], concat([commitment, u32(combinations)]))
}

export function revealCouponIx ({ player, admin, nonce, index, picks, revealNonce }) {
  const event = eventPda(admin, nonce)
  return ix('reveal_coupon', [
    signer(player, false), writable(couponPda(event, index)), readonly(event)
  ], concat([picks, u64(revealNonce)]))
}

export function resolveEventIx ({ admin, event, other, coupon }) {
  const accounts = [
    writable(event), readonly(signersPda(event)), readonly(vaultPda(event)), writable(schedulePda(admin)),
    signer(admin, false), signer(other, false)
  ]
  if (coupon) accounts.push(readonly(coupon))
  const results = Uint8Array.from([...'1'.repeat(7)].map(char => char.charCodeAt(0)))
  return ix('resolve_event', accounts, concat([u32(results.length), results, Uint8Array.of(2)]))
}

export async function couponCount (rpcUrl, eventAddress) {
  const data = await getAccount(rpcUrl, eventAddress)
  if (!data || data.length < COUPON_COUNT_OFFSET + 4) throw new Error('The on-chain event was not found')
  return new DataView(data.buffer, data.byteOffset, data.byteLength).getUint32(COUPON_COUNT_OFFSET, true)
}

export async function sendProgramTransaction (rpcUrl, feePayer, instructions) {
  const { value } = await latestBlockhash(rpcUrl)
  const { message } = compileMessage(feePayer, instructions, value.blockhash)
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
