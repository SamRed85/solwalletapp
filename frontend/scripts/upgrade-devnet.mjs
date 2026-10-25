// Deploy or upgrade the devnet program. The deployer key is the loader authority.
// The program must not upgrade itself: Solana rejects that with PrivilegeEscalation.
//
//   node scripts/upgrade-devnet.mjs --upgrade

import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Decode, base58Encode } from '../src/services/solana.js'
import { compileMessage, PROGRAM_ID } from '../src/services/program.js'

const ENDPOINTS = [process.env.SOLANA_RPC_URL || 'https://api.devnet.solana.com']
const LOADER = 'BPFLoaderUpgradeab1e11111111111111111111111'
const SYSTEM = '11111111111111111111111111111111'
const RENT = 'SysvarRent111111111111111111111111111111111'
const CLOCK = 'SysvarC1ock11111111111111111111111111111111'
const DEPLOYER = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'
const COUNCIL_SIGNER = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
const COUNCIL = 'D6e74RVzp1cTXKQb1UyCygJg17szm9uvphiKZTKfarpt'
const BUFFER_HEADER = 37
const PROGRAM_DATA_HEADER = 45

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../programs/predictions')
const elf = readFileSync(resolve(root, 'target/deploy/predictions.so'))
const mode = process.argv[2]
if (!['--probe', '--upgrade'].includes(mode)) {
  console.log('Use --probe to check the live upgrade instruction, or --upgrade to publish target/deploy/predictions.so')
  process.exit(1)
}

function loadKey (name, expected) {
  const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(root, 'keys', name), 'utf8')))
  const seed = secret.slice(0, 32)
  const address = base58Encode(ed25519.getPublicKey(seed))
  if (expected && address !== expected) throw new Error(`${name} is ${address}, expected ${expected}`)
  return { seed, address }
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

function concat (parts) {
  const out = new Uint8Array(parts.reduce((total, part) => total + part.length, 0))
  let offset = 0
  for (const part of parts) {
    out.set(part, offset)
    offset += part.length
  }
  return out
}

const disc = name => createHash('sha256').update(`global:${name}`).digest().subarray(0, 8)
const writable = pubkey => ({ pubkey, isSigner: false, isWritable: true })
const readonly = pubkey => ({ pubkey, isSigner: false, isWritable: false })
const signer = (pubkey, isWritable) => ({ pubkey, isSigner: true, isWritable })

function programDataAddress (program) {
  const programBytes = base58Decode(program, 32)
  const loader = base58Decode(LOADER, 32)
  for (let bump = 255; bump >= 0; bump--) {
    const hash = createHash('sha256').update(Buffer.concat([programBytes, Uint8Array.of(bump), loader, Buffer.from('ProgramDerivedAddress')])).digest()
    try {
      ed25519.ExtendedPoint.fromHex(hash)
    } catch {
      return base58Encode(hash)
    }
  }
  throw new Error('No program data address')
}

let endpoint = 0

async function rpc (method, params) {
  let last
  for (let attempt = 0; attempt < ENDPOINTS.length; attempt++) {
    const url = ENDPOINTS[(endpoint + attempt) % ENDPOINTS.length]
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
    })
    const body = await response.json()
    if (!body.error) return body.result
    last = body.error
    const limited = /429|Too many requests|rate limit/i.test(JSON.stringify(body.error))
    if (!limited) break
    endpoint = (endpoint + 1) % ENDPOINTS.length
  }
  const error = new Error(`${method} failed: ${JSON.stringify(last)}`)
  error.rpc = last
  throw error
}

const sleep = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))
const limited = error => /429|Too many requests|rate limit/i.test(String(error))

function signedTransaction (signers, ixList, blockhash) {
  const compiled = compileMessage(DEPLOYER, ixList, blockhash)
  const signatures = compiled.signers.map(address => {
    const signerKey = signers.find(key => key.address === address)
    if (!signerKey) throw new Error(`No signer for ${address}`)
    return ed25519.sign(compiled.message, signerKey.seed)
  })
  return concat([Uint8Array.of(signatures.length), ...signatures, compiled.message])
}

let cachedHash = null

async function recentBlockhash () {
  if (cachedHash && Date.now() < cachedHash.until) return cachedHash.blockhash
  const value = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
  cachedHash = { blockhash: value.blockhash, until: Date.now() + 20000 }
  return value.blockhash
}

async function send (signers, ixList) {
  for (let attempt = 0; attempt < 8; attempt++) {
    try {
      const blockhash = await recentBlockhash()
      const transaction = signedTransaction(signers, ixList, blockhash)
      const signature = await rpc('sendTransaction', [
        Buffer.from(transaction).toString('base64'),
        { encoding: 'base64', skipPreflight: true, preflightCommitment: 'processed' }
      ])
      for (let check = 0; check < 20; check++) {
        const status = (await rpc('getSignatureStatuses', [[signature]])).value[0]
        if (status?.err) throw new Error(`transaction ${signature} failed: ${JSON.stringify(status.err)}`)
        if (status?.confirmationStatus) return signature
        await sleep(400)
      }
      cachedHash = null
      throw new Error(`transaction ${signature} was not confirmed`)
    } catch (error) {
      if (/blockhash/i.test(String(error))) cachedHash = null
      if ((!limited(error) && !/not confirmed/.test(String(error))) || attempt === 7) throw error
      await sleep(800 * (attempt + 1))
    }
  }
}

function councilIx (name, accounts, signerCount, signers) {
  return {
    programId: PROGRAM_ID,
    accounts: [...accounts, ...signers.map(key => signer(key.address, false))],
    data: concat([disc(name), Uint8Array.of(signerCount)])
  }
}

const deployer = loadKey('deployer-keypair.json', DEPLOYER)
const member = loadKey('council-signer.json', COUNCIL_SIGNER)
const programData = programDataAddress(PROGRAM_ID)
const councilSigners = [deployer, member]

function scheduleIx (buffer) {
  return councilIx('schedule_upgrade', [writable(COUNCIL), readonly(buffer), readonly(DEPLOYER)], 2, councilSigners)
}

function overrideIx () {
  return councilIx('override_upgrade_delay', [writable(COUNCIL)], 2, councilSigners)
}

function loaderUpgradeIx (buffer) {
  return {
    programId: LOADER,
    accounts: [
      writable(programData), writable(PROGRAM_ID), writable(buffer), writable(DEPLOYER),
      readonly(RENT), readonly(CLOCK), signer(DEPLOYER, false)
    ],
    data: u32(3)
  }
}

function extendProgramIx (additional) {
  return {
    programId: LOADER,
    accounts: [
      writable(programData), writable(PROGRAM_ID), readonly(SYSTEM), signer(DEPLOYER, true)
    ],
    data: concat([u32(6), u32(additional)])
  }
}

if (mode === '--probe') {
  console.log(`probing execute_upgrade with program data ${programData}`)
  console.log(`schedule ${await send(councilSigners, [scheduleIx(DEPLOYER)])}`)
  console.log(`override ${await send(councilSigners, [overrideIx()])}`)
  try {
    console.log(`execute ${await send([deployer], [executeIx(DEPLOYER)])}`)
  } catch (error) {
    console.log(String(error.message).slice(0, 4000))
  }
  process.exit(0)
}

const programKey = loadKey('program-keypair.json', PROGRAM_ID)
const maxDataLen = elf.length + 20480
console.log(`publishing ${PROGRAM_ID}, ${elf.length} bytes, capacity ${maxDataLen}`)

const bufferPath = resolve(root, 'keys/buffer-keypair.json')
if (!existsSync(bufferPath)) {
  const seed = ed25519.utils.randomPrivateKey()
  const pubkey = ed25519.getPublicKey(seed)
  writeFileSync(bufferPath, JSON.stringify(Array.from(concat([seed, pubkey]))))
}
const buffer = loadKey('buffer-keypair.json')
const space = BUFFER_HEADER + elf.length
const existing = await rpc('getAccountInfo', [buffer.address, { encoding: 'base64', commitment: 'confirmed' }])
if (!existing.value) {
  const lamports = await rpc('getMinimumBalanceForRentExemption', [space])
  console.log(`creating buffer ${buffer.address} for ${lamports} lamports`)
  console.log(await send([deployer, buffer], [
    {
      programId: SYSTEM,
      accounts: [signer(DEPLOYER, true), signer(buffer.address, true)],
      data: concat([u32(0), u64(lamports), u64(space), base58Decode(LOADER, 32)])
    },
    {
      programId: LOADER,
      accounts: [writable(buffer.address), readonly(DEPLOYER)],
      data: u32(0)
    }
  ]))
}

function writeIx (offset, bytes) {
  return {
    programId: LOADER,
    accounts: [writable(buffer.address), signer(DEPLOYER, false)],
    data: concat([u32(1), u32(offset), u64(bytes.length), Uint8Array.from(bytes)])
  }
}

function chunkSize () {
  const blockhash = base58Encode(new Uint8Array(32))
  for (let size = 900; size >= 200; size -= 20) {
    const tx = signedTransaction([deployer], [writeIx(0, new Uint8Array(size))], blockhash)
    if (tx.length <= 1232) return size
  }
  throw new Error('a buffer write does not fit')
}

async function bufferBody () {
  const account = await rpc('getAccountInfo', [buffer.address, { encoding: 'base64', commitment: 'confirmed' }])
  if (!account.value) throw new Error('buffer disappeared')
  const data = Buffer.from(account.value.data[0], 'base64')
  if (data.length !== space) throw new Error(`buffer is ${data.length} bytes, expected ${space}`)
  return data.subarray(BUFFER_HEADER)
}

const size = chunkSize()
let body = await bufferBody()
const pieces = []
for (let offset = 0; offset < elf.length; offset += size) {
  const bytes = elf.subarray(offset, Math.min(elf.length, offset + size))
  if (!body.subarray(offset, offset + bytes.length).equals(bytes)) pieces.push({ offset, bytes })
}
console.log(`writing ${pieces.length} chunks of up to ${size} bytes`)
for (let index = 0; index < pieces.length; index++) {
  const piece = pieces[index]
  await send([deployer], [writeIx(piece.offset, piece.bytes)])
  if (index === 0 || index + 1 === pieces.length || (index + 1) % 25 === 0) {
    console.log(`wrote ${index + 1}/${pieces.length}`)
  }
}
body = await bufferBody()
const onChain = createHash('sha256').update(body).digest('hex')
const local = createHash('sha256').update(elf).digest('hex')
if (onChain !== local) throw new Error(`buffer hash ${onChain} does not match ${local}`)
console.log(`buffer matches ${local}`)

const programInfo = await rpc('getAccountInfo', [PROGRAM_ID, { encoding: 'base64', commitment: 'confirmed' }])
if (!programInfo.value) {
  const programLamports = await rpc('getMinimumBalanceForRentExemption', [36])
  console.log(`deploying program account for ${programLamports} lamports`)
  console.log(await send([deployer, programKey], [
    {
      programId: SYSTEM,
      accounts: [signer(DEPLOYER, true), signer(PROGRAM_ID, true)],
      data: concat([u32(0), u64(programLamports), u64(36), base58Decode(LOADER, 32)])
    },
    {
      programId: LOADER,
      accounts: [
        signer(DEPLOYER, true), writable(programData), writable(PROGRAM_ID), writable(buffer.address),
        readonly(RENT), readonly(CLOCK), readonly(SYSTEM), signer(DEPLOYER, false)
      ],
      data: concat([u32(2), u64(maxDataLen)])
    }
  ]))
}

let dumped = await rpc('getAccountInfo', [programData, { encoding: 'base64', commitment: 'confirmed' }])
if (!dumped.value) throw new Error('program data account is missing after deploy')
let raw = Buffer.from(dumped.value.data[0], 'base64')
const authority = raw[12] === 1 ? base58Encode(raw.subarray(13, 45)) : ''
if (authority !== DEPLOYER) throw new Error(`upgrade authority is ${authority}`)
const needed = PROGRAM_DATA_HEADER + elf.length
if (raw.length < needed) {
  const additional = needed - raw.length
  const slot = await rpc('getSlot', [{ commitment: 'confirmed' }])
  console.log(`extending program data by ${additional} bytes`)
  console.log(await send([deployer], [extendProgramIx(additional)]))
  for (let wait = 0; wait < 40; wait++) {
    const now = await rpc('getSlot', [{ commitment: 'confirmed' }])
    if (now > slot) break
    await sleep(500)
  }
  dumped = await rpc('getAccountInfo', [programData, { encoding: 'base64', commitment: 'confirmed' }])
  raw = Buffer.from(dumped.value.data[0], 'base64')
  if (raw.length < needed) throw new Error(`program data is ${raw.length} bytes after extend, need ${needed}`)
}
const deployedHashOf = account => {
  const body = Buffer.from(account.value.data[0], 'base64').subarray(PROGRAM_DATA_HEADER, PROGRAM_DATA_HEADER + elf.length)
  return createHash('sha256').update(body).digest('hex')
}
let deployedHash = deployedHashOf(dumped)
if (deployedHash !== local) {
  console.log(`loader upgrade ${await send([deployer], [loaderUpgradeIx(buffer.address)])}`)
  dumped = await rpc('getAccountInfo', [programData, { encoding: 'base64', commitment: 'confirmed' }])
  deployedHash = deployedHashOf(dumped)
}
if (deployedHash !== local) throw new Error(`deployed program hash ${deployedHash} does not match ${local}`)
writeFileSync(resolve(root, 'build-hash.txt'), `${local}\n`)
console.log(`devnet program matches ${local}`)
