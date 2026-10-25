// Two-wallet vault run and two-wallet treasury run on devnet.
// Players are temporary keys funded by the deployer. Nothing here is saved to disk.

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { sha256 } from '@noble/hashes/sha256'
import { base58Encode } from '../src/services/solana.js'
import {
  SYSTEM_PROGRAM, approveResolutionIx, claimIx, compileMessage, couponCommitment, couponPda, createEventIx, decodeEvent,
  eventPda, picksFromSystem, placeCouponIx, PROGRAM_ID, revealCouponIx, tallyIx, vaultPda
} from '../src/services/program.js'

const here = dirname(fileURLToPath(import.meta.url))
const keysDir = resolve(here, '../../programs/predictions/keys')
const API = 'http://127.0.0.1:8080'
const TREASURY = '4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy'
const DEPLOYER = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'
const COUNCIL = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
const CLOCK = 'SysvarC1ock11111111111111111111111111111111'
const REVEAL_NONCE = 7
const FUND = 30_000_000

function rpcUrl () {
  if (process.env.SOLANA_RPC_URL) return process.env.SOLANA_RPC_URL
  const file = readFileSync(resolve(here, '../../.env'), 'utf8')
  const line = file.split(/\r?\n/).find(item => item.startsWith('SOLANA_RPC_URL='))
  if (!line) throw new Error('SOLANA_RPC_URL is missing from .env')
  return line.slice('SOLANA_RPC_URL='.length).trim().replace(/^["']|["']$/g, '')
}

const url = rpcUrl()

function loadKey (name, expected) {
  const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(keysDir, name), 'utf8')))
  const seed = secret.slice(0, 32)
  const address = base58Encode(ed25519.getPublicKey(seed))
  if (expected && address !== expected) throw new Error(`${name} does not match the expected address`)
  return { seed, address }
}

function playerKey () {
  const seed = ed25519.utils.randomPrivateKey()
  return { seed, address: base58Encode(ed25519.getPublicKey(seed)) }
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
  for (let attempt = 0; attempt < 8; attempt++) {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
    })
    const body = await response.json()
    if ((response.status === 429 || /429|Too many requests/i.test(JSON.stringify(body.error || ''))) && attempt < 7) {
      await new Promise(resolve => setTimeout(resolve, 2000 * (attempt + 1)))
      continue
    }
    if (body.error) throw new Error(`${method} failed: ${JSON.stringify(body.error)}`)
    return body.result
  }
  throw new Error(`${method} was rate limited`)
}

async function confirm (signature, want = 'confirmed') {
  for (let attempt = 0; attempt < 90; attempt++) {
    const status = (await rpc('getSignatureStatuses', [[signature], { searchTransactionHistory: true }])).value[0]
    if (status?.err) throw new Error(`transaction ${signature} failed: ${JSON.stringify(status.err)}`)
    if (status?.confirmationStatus === 'finalized' || (want === 'confirmed' && status?.confirmationStatus === 'confirmed')) return status.confirmationStatus
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error(`transaction ${signature} was not ${want}`)
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

function transferIx (from, to, lamports) {
  const data = new Uint8Array(12)
  const view = new DataView(data.buffer)
  view.setUint32(0, 2, true)
  view.setBigUint64(4, BigInt(lamports), true)
  return {
    programId: SYSTEM_PROGRAM,
    accounts: [
      { pubkey: from, isSigner: true, isWritable: true },
      { pubkey: to, isSigner: false, isWritable: true }
    ],
    data
  }
}

async function unixTime () {
  const account = await rpc('getAccountInfo', [CLOCK, { encoding: 'base64', commitment: 'confirmed' }])
  const data = Buffer.from(account.value.data[0], 'base64')
  return Number(data.readBigInt64LE(32))
}

async function waitUntil (timestamp) {
  for (let attempt = 0; attempt < 120; attempt++) {
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

function must (value, label) {
  if (value instanceof Error) throw new Error(`${label}: ${value.message}`)
  return value
}

async function vaultRun (admin, council, payer) {
  console.log('vault: funding the event admin and two players')
  const winner = playerKey()
  const loser = playerKey()
  const adminBalance = await balance(admin.address)
  const topUp = adminBalance < 150_000_000 ? [transferIx(payer.address, admin.address, 200_000_000)] : []
  must(await send(payer, [payer], [...topUp, transferIx(payer.address, winner.address, FUND), transferIx(payer.address, loser.address, FUND)]), 'fund players')

  const now = await unixTime()
  const deadline = now + 35
  const revealDeadline = deadline + 40
  const nonce = BigInt(Date.now())
  const event = eventPda(admin.address, nonce)
  const vault = vaultPda(event)
  const winPicks = picksFromSystem(Array.from({ length: 7 }, () => ['1']))
  const losePicks = picksFromSystem(Array.from({ length: 7 }, () => ['2']))
  const winCoupon = couponPda(event, 0)
  const loseCoupon = couponPda(event, 1)

  console.log('vault: creating the event')
  must(await send(admin, [admin], [createEventIx({ admin: admin.address, other: council.address, nonce, deadline, revealDeadline })]), 'create event')
  must(await send(winner, [winner], [placeCouponIx({ player: winner.address, admin: admin.address, nonce, index: 0, commitment: couponCommitment(winner.address, winPicks, REVEAL_NONCE), combinations: 1 })]), 'winner entry')
  must(await send(loser, [loser], [placeCouponIx({ player: loser.address, admin: admin.address, nonce, index: 1, commitment: couponCommitment(loser.address, losePicks, REVEAL_NONCE), combinations: 1 })]), 'loser entry')

  await waitUntil(deadline)
  const late = await send(winner, [winner], [placeCouponIx({ player: winner.address, admin: admin.address, nonce, index: 2, commitment: couponCommitment(winner.address, winPicks, REVEAL_NONCE), combinations: 1 })])
  if (!(late instanceof Error)) throw new Error('a late entry must be rejected')

  must(await send(winner, [winner], [revealCouponIx({ player: winner.address, admin: admin.address, nonce, index: 0, picks: winPicks, revealNonce: REVEAL_NONCE })]), 'winner reveal')
  must(await send(loser, [loser], [revealCouponIx({ player: loser.address, admin: admin.address, nonce, index: 1, picks: losePicks, revealNonce: REVEAL_NONCE })]), 'loser reveal')
  await waitUntil(revealDeadline)

  const results = '1111111'
  must(await send(admin, [admin], [approveResolutionIx({ signer: admin.address, admin: admin.address, event, results })]), 'first approval')
  must(await send(admin, [admin, council], [approveResolutionIx({ signer: council.address, admin: admin.address, event, results })]), 'second approval')
  must(await send(admin, [admin], [tallyIx({ admin: admin.address, event, coupons: [winCoupon, loseCoupon] })]), 'tally')

  const settled = decodeEvent(await account(event))
  if (settled.status !== 'resolved' || settled.perWinner[6] !== 1_900_000n || settled.house !== 100_000n) {
    throw new Error(`unexpected vault settlement perWinner=${settled.perWinner[6]} house=${settled.house} status=${settled.status}`)
  }

  const redirected = await send(loser, [loser], [claimIx({ owner: loser.address, event, coupon: winCoupon, admin: admin.address })])
  if (!(redirected instanceof Error)) throw new Error('a claim cannot be redirected')
  must(await send(loser, [loser], [claimIx({ owner: loser.address, event, coupon: loseCoupon, admin: admin.address })]), 'loser claim')
  if (await account(loseCoupon)) throw new Error('the losing coupon account must close')
  const before = await balance(winner.address)
  must(await send(winner, [winner], [claimIx({ owner: winner.address, event, coupon: winCoupon, admin: admin.address })]), 'winner claim')
  const after = await balance(winner.address)
  if (after <= before) throw new Error('the vault must pay the winner')
  if (await account(winCoupon)) throw new Error('the winning coupon account must close')
  const repeated = await send(winner, [winner], [claimIx({ owner: winner.address, event, coupon: winCoupon, admin: admin.address })])
  if (!(repeated instanceof Error)) throw new Error('a claim cannot be repeated')
  if (BigInt(await balance(vault)) !== settled.rentReserve + settled.house) throw new Error('the vault did not keep the house share')

  console.log(`vault: winner ${winner.address} gained ${after - before} lamports`)
  console.log(`vault: loser ${loser.address} closed without a prize`)
  console.log(`vault: event ${event}`)
}

function paymentMemo (eventId, system) {
  const sum = sha256(new TextEncoder().encode(`${eventId}\n${system.join(',')}`))
  return `sol-entry-v1:${Buffer.from(sum).toString('hex')}`
}

async function api (cookieJar) {
  return async (method, path, body) => {
    const response = await fetch(API + path, {
      method,
      headers: { 'content-type': 'application/json', cookie: cookieJar.cookie },
      body: body === undefined ? undefined : JSON.stringify(body)
    })
    for (const item of response.headers.getSetCookie?.() || []) {
      const pair = item.split(';')[0]
      if (pair.startsWith('sol_session=')) cookieJar.cookie = pair
    }
    const text = await response.text()
    if (!response.ok) throw new Error(`${method} ${path} -> ${response.status} ${text}`)
    return text ? JSON.parse(text) : null
  }
}

async function login (key) {
  const jar = { cookie: '' }
  const call = await api(jar)
  const { nonce, message } = await call('POST', '/api/auth/nonce', { publicKey: key.address })
  const signature = Buffer.from(ed25519.sign(new TextEncoder().encode(message), key.seed)).toString('base64')
  await call('POST', '/api/auth/login', { publicKey: key.address, nonce, signature })
  return call
}

async function treasuryRun (admin, deployer) {
  console.log('treasury: creating an event')
  const call = await login(admin)
  const start = new Date(Date.now() + 6 * 60 * 1000)
  const end = new Date(start.getTime() + 60 * 60 * 1000)
  const event = await call('POST', '/api/events', {
    title: 'Multi-wallet test',
    description: 'Automated two-wallet treasury run',
    gameCount: 7,
    couponPriceLamports: 1_000_000,
    tiers: [{ correct: 7, percent: 95 }],
    startTime: start.toISOString(),
    endTime: end.toISOString()
  })
  const games = Array.from({ length: 7 }, (_, index) => ({
    homeTeam: `Home ${index + 1}`,
    awayTeam: `Away ${index + 1}`,
    kickoff: start.toISOString()
  }))
  await call('PUT', `/api/events/${event.id}/games`, { games })

  const winner = playerKey()
  const loser = playerKey()
  must(await send(deployer, [deployer], [transferIx(deployer.address, winner.address, FUND), transferIx(deployer.address, loser.address, FUND)]), 'fund treasury players')
  const winnerSystem = Array.from({ length: 7 }, () => '1')
  const loserSystem = Array.from({ length: 7 }, () => '2')

  async function pay (key, system) {
    const memo = paymentMemo(event.id, system)
    const { blockhash } = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
    const memoBytes = new TextEncoder().encode(memo)
    const memoIx = { programId: 'MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr', accounts: [], data: memoBytes }
    const message = compileMessage(key.address, [transferIx(key.address, TREASURY, 1_000_000), memoIx], blockhash)
    const signatureBytes = ed25519.sign(message.message, key.seed)
    const transaction = concat([Uint8Array.of(1), signatureBytes, message.message])
    const signature = await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])
    console.log(`treasury: waiting for ${key.address.slice(0, 4)}… payment to finalize`)
    await confirm(signature, 'finalized')
    const player = await login(key)
    const entry = await player('POST', `/api/events/${event.id}/entries`, { system, signature })
    return entry
  }

  await pay(winner, winnerSystem)
  await pay(loser, loserSystem)
  const again = await login(winner)
  const duplicate = await again('POST', `/api/events/${event.id}/entries`, { system: winnerSystem, signature: 'unused' }).catch(error => error)
  if (!(duplicate instanceof Error)) throw new Error('an unpaid signature must be rejected')

  console.log(`treasury: waiting until ${start.toISOString()} to settle`)
  while (Date.now() < start.getTime() + 2000) await new Promise(resolve => setTimeout(resolve, 5000))
  const settled = await call('POST', `/api/events/${event.id}/results`, { results: winnerSystem })
  const repeat = await call('POST', `/api/events/${event.id}/results`, { results: winnerSystem }).catch(error => error)
  if (!(repeat instanceof Error)) throw new Error('results must be set once')
  const report = await call('GET', `/api/events/${event.id}/payouts`)
  const won = report.payouts.find(row => row.walletAddress === winner.address)
  const lost = report.payouts.find(row => row.walletAddress === loser.address)
  if (!won || won.winningsLamports !== 1_900_000) throw new Error(`winner payout ${JSON.stringify(won)}`)
  if (lost) throw new Error('a wallet with no winning rows must be omitted')
  if (settled.settlement?.houseLamports !== 100_000 || report.settlement?.paidLamports !== 1_900_000) {
    throw new Error(`settlement house ${settled.settlement?.houseLamports} paid ${report.settlement?.paidLamports}`)
  }
  console.log(`treasury: event ${event.id}`)
  console.log(`treasury: winner ${winner.address} is owed 1900000 lamports`)
  console.log(`treasury: loser ${loser.address} is owed nothing`)
}

const health = await fetch(API + '/health').then(response => response.json()).catch(() => null)
if (health?.status !== 'healthy') throw new Error('the backend is not running on port 8080')

const deployer = loadKey('deployer-keypair.json', DEPLOYER)
const council = loadKey('council-signer.json', COUNCIL)
const treasury = loadKey('admin-keypair.json', TREASURY)
if (await balance(deployer.address) < 100_000_000) throw new Error('the deployer needs at least 0.1 SOL to fund the test wallets')

console.log(`program ${PROGRAM_ID}`)
await vaultRun(treasury, council, deployer)
await treasuryRun(treasury, deployer)
console.log('multi-wallet treasury and vault runs passed')
