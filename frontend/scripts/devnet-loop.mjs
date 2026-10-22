// Place, win, and claim on devnet with the deployer key. No airdrop. The backend is not involved.
// The other result signer is the council key on disk.

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Encode } from '../src/services/solana.js'
import {
  claimIx, compileMessage, couponCommitment, couponPda, createEventIx, eventPda,
  picksFromSystem, placeCouponIx, PROGRAM_ID, resolveEventIx, revealCouponIx, vaultPda
} from '../src/services/program.js'

const url = 'https://api.devnet.solana.com'
const CLOCK = 'SysvarC1ock11111111111111111111111111111111'
const DEPLOYER = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'
const COUNCIL_SIGNER = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
const REVEAL_NONCE = 7
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../programs/predictions/keys')

function loadKey (name, expected) {
  const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(root, name), 'utf8')))
  const seed = secret.slice(0, 32)
  const address = base58Encode(ed25519.getPublicKey(seed))
  if (address !== expected) throw new Error(`${name} is ${address}`)
  return { seed, address }
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
    await new Promise(resolve => setTimeout(resolve, 1000))
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
  const transaction = concat([Uint8Array.of(signatures.length), ...signatures, compiled.message])
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

const admin = loadKey('deployer-keypair.json', DEPLOYER)
const other = loadKey('council-signer.json', COUNCIL_SIGNER)
const now = await unixTime()
const deadline = now + 20
const revealDeadline = deadline + 45
const nonce = BigInt(Date.now())
const event = eventPda(admin.address, nonce)
const vault = vaultPda(event)
const coupon = couponPda(event, 0)
const picks = picksFromSystem(Array.from({ length: 7 }, () => ['1']))
const commitment = couponCommitment(admin.address, picks, REVEAL_NONCE)

const created = await send(admin, [admin], [createEventIx({ admin: admin.address, other: other.address, nonce, deadline, revealDeadline })])
if (created instanceof Error) throw created
const placed = await send(admin, [admin], [placeCouponIx({ player: admin.address, admin: admin.address, nonce, index: 0, commitment, combinations: 1 })])
if (placed instanceof Error) throw placed

await waitUntil(deadline)
const late = await send(admin, [admin], [placeCouponIx({ player: admin.address, admin: admin.address, nonce, index: 1, commitment, combinations: 1 })])
if (!(late instanceof Error)) throw new Error('a late entry must be rejected')

const revealed = await send(admin, [admin], [revealCouponIx({ player: admin.address, admin: admin.address, nonce, index: 0, picks, revealNonce: REVEAL_NONCE })])
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

const before = await balance(admin.address)
const claimed = await send(admin, [admin], [claimIx({ owner: admin.address, event, coupon })])
if (claimed instanceof Error) throw claimed
const after = await balance(admin.address)
if (after <= before) throw new Error('the vault must pay the winner')
if (await account(coupon)) throw new Error('a paid coupon account must close')
const repeated = await send(admin, [admin], [claimIx({ owner: admin.address, event, coupon })])
if (!(repeated instanceof Error)) throw new Error('a claim cannot be repeated')
const reserve = eventData.readBigUInt64LE(8 + 273)
if (BigInt(await balance(vault)) !== reserve + house) throw new Error('the vault did not keep the house share')

console.log('devnet wallet placed, won, and claimed without the backend')
console.log(`player gained ${after - before} lamports`)
console.log(`program ${PROGRAM_ID}`)
console.log(`event ${event}`)
console.log(`claim ${claimed}`)
