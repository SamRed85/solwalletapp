import { ed25519 } from '@noble/curves/ed25519'
import { base58Encode } from '../src/services/solana.js'
import {
  claimIx, compileMessage, couponCommitment, couponPda, createEventIx, escrowPda, eventPda,
  findPda, picksFromSystem, placeCouponIx, PROGRAM_ID, resolveEventIx, revealCouponIx, vaultPda
} from '../src/services/program.js'

const url = process.env.SOLANA_URL || 'http://127.0.0.1:8899'
const CLOCK = 'SysvarC1ock11111111111111111111111111111111'
const REVEAL_NONCE = 7

function keypair () {
  const seed = ed25519.utils.randomPrivateKey()
  const pubkey = ed25519.getPublicKey(seed)
  return { seed, address: base58Encode(pubkey) }
}

function concat (parts) {
  const out = new Uint8Array(parts.reduce((total, part) => total + part.length, 0))
  let offset = 0
  for (const part of parts) {
    out.set(part, offset)
    offset += part.length
  }
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

async function rpc (method, params) {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
  })
  const body = await response.json()
  if (body.error) throw new Error(`${method} failed: ${JSON.stringify(body.error)}`)
  return body.result
}

async function confirm (signature) {
  for (let attempt = 0; attempt < 40; attempt++) {
    const status = (await rpc('getSignatureStatuses', [[signature], { searchTransactionHistory: true }])).value[0]
    if (status?.err) throw new Error(`transaction ${signature} failed: ${JSON.stringify(status.err)}`)
    if (status?.confirmationStatus === 'confirmed' || status?.confirmationStatus === 'finalized') return
    await new Promise(resolve => setTimeout(resolve, 500))
  }
  throw new Error(`transaction ${signature} was not confirmed`)
}

async function send (feePayer, signers, instructions) {
  const { blockhash } = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
  const compiled = compileMessage(feePayer.address, instructions, blockhash)
  const signatures = compiled.signers.map(address => {
    const signer = signers.find(key => key.address === address)
    if (!signer) throw new Error(`No signer for ${address}`)
    return ed25519.sign(compiled.message, signer.seed)
  })
  const transaction = concat([compactU16(signatures.length), ...signatures, compiled.message])
  try {
    const signature = await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])
    await confirm(signature)
    return signature
  } catch (error) {
    return error
  }
}

async function unixTime () {
  const account = await rpc('getAccountInfo', [CLOCK, { encoding: 'base64', commitment: 'confirmed' }])
  const data = Buffer.from(account.value.data[0], 'base64')
  return Number(data.readBigInt64LE(32))
}

async function waitUntil (timestamp) {
  for (let attempt = 0; attempt < 90; attempt++) {
    if (await unixTime() >= timestamp) return
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error(`clock did not reach ${timestamp}`)
}

async function balance (address) {
  return (await rpc('getBalance', [address, { commitment: 'confirmed' }])).value
}

async function account (address) {
  const result = await rpc('getAccountInfo', [address, { encoding: 'base64', commitment: 'confirmed' }])
  return result.value ? Buffer.from(result.value.data[0], 'base64') : null
}

for (let attempt = 0; attempt < 60; attempt++) {
  try {
    await rpc('getVersion', [])
    break
  } catch (error) {
    if (attempt === 59) throw error
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
}

const council = findPda([new TextEncoder().encode('upgrade-council')])
if (council !== '9ad7bFZNUVuvGkM2ZbepzK6wADV4LaWpW2weQPxXSWD2') {
  throw new Error(`PDA derivation drifted: ${council}`)
}

const admin = keypair()
const other = keypair()
const player = keypair()
const stranger = keypair()
for (const key of [admin, other, player, stranger]) {
  await confirm(await rpc('requestAirdrop', [key.address, 1_000_000_000]))
}

const now = await unixTime()
const deadline = now + 12
const revealDeadline = deadline + 40
const nonce = BigInt(Date.now())
const event = eventPda(admin.address, nonce)
const vault = vaultPda(event)
const coupon = couponPda(event, 0)
const picks = picksFromSystem(Array.from({ length: 7 }, () => ['1']))
const commitment = couponCommitment(player.address, picks, REVEAL_NONCE)

const created = await send(admin, [admin], [createEventIx({ admin: admin.address, other: other.address, nonce, deadline, revealDeadline })])
if (created instanceof Error) throw created
const placed = await send(player, [player], [placeCouponIx({ player: player.address, admin: admin.address, nonce, index: 0, commitment, combinations: 1 })])
if (placed instanceof Error) throw placed

await waitUntil(deadline)
const late = await send(player, [player], [placeCouponIx({ player: player.address, admin: admin.address, nonce, index: 1, commitment, combinations: 1 })])
if (!(late instanceof Error)) throw new Error('a late entry must be rejected')

const revealed = await send(player, [player], [revealCouponIx({ player: player.address, admin: admin.address, nonce, index: 0, picks, revealNonce: REVEAL_NONCE })])
if (revealed instanceof Error) throw revealed
await waitUntil(revealDeadline)

const missing = await send(admin, [admin, other], [resolveEventIx({ admin: admin.address, other: other.address, event })])
if (!(missing instanceof Error)) throw new Error('resolution must include every coupon')
const resolved = await send(admin, [admin, other], [resolveEventIx({ admin: admin.address, other: other.address, event, coupon })])
if (resolved instanceof Error) throw resolved

const eventData = await account(event)
const house = eventData.readBigUInt64LE(8 + 142)
const perWinner = eventData.readBigUInt64LE(8 + 167 + 56)
if (perWinner !== 950_000n || house !== 50_000n) {
  throw new Error(`unexpected settlement perWinner=${perWinner} house=${house}`)
}

const redirected = await send(stranger, [stranger], [claimIx({ owner: stranger.address, event, coupon })])
if (!(redirected instanceof Error)) throw new Error('a claim cannot be redirected')
const before = await balance(player.address)
const claimed = await send(player, [player], [claimIx({ owner: player.address, event, coupon })])
if (claimed instanceof Error) throw claimed
const after = await balance(player.address)
if (after <= before) throw new Error('the vault must pay the winner')
if (await account(coupon)) throw new Error('a paid coupon account must close')
const repeated = await send(player, [player], [claimIx({ owner: player.address, event, coupon })])
if (!(repeated instanceof Error)) throw new Error('a claim cannot be repeated')
const reserve = eventData.readBigUInt64LE(8 + 273)
if (BigInt(await balance(vault)) !== reserve + house) throw new Error('the vault did not keep the house share')

console.log('wallet client placed, won, and claimed against the built program')
console.log(`player gained ${after - before} lamports`)
console.log(`program ${PROGRAM_ID}`)
console.log(`event ${event}`)
console.log(`escrow ${escrowPda()}`)
