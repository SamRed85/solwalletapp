// Finish a devnet deploy from the already funded buffer with one transaction.
// `solana program deploy` rewrites the buffer and the public RPC rejects that burst.
// This script reads the buffer once, compares it to predictions.so, and only then
// sends CreateAccount + DeployWithMaxDataLen. It never writes buffer bytes.
//
//   node scripts/finish-devnet.mjs --prepare
//   node scripts/finish-devnet.mjs --check
//   node scripts/finish-devnet.mjs --repair
//   node scripts/finish-devnet.mjs --send

import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Decode, base58Encode } from '../src/services/solana.js'
import { compileMessage, findPda, PROGRAM_ID } from '../src/services/program.js'

const RPC = 'https://api.devnet.solana.com'
const LOADER = 'BPFLoaderUpgradeab1e11111111111111111111111'
const SYSTEM = '11111111111111111111111111111111'
const RENT = 'SysvarRent111111111111111111111111111111111'
const CLOCK = 'SysvarC1ock11111111111111111111111111111111'
const DEPLOYER = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'
const BUFFER = '7d2sVq63vaUJqEdtMmbkEmnmtE1QWphduXyAQeV3LLFt'
const COUNCIL = findPda([new TextEncoder().encode('upgrade-council')])
const COUNCIL_SIGNER = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
const BUFFER_HEADER = 37
const PROGRAM_SPACE = 36

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../programs/predictions')
const elf = readFileSync(resolve(root, 'target/deploy/predictions.so'))

function loadKey (name, expected) {
  const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(root, 'keys', name), 'utf8')))
  if (secret.length < 32) throw new Error(`${name} is not a Solana keypair`)
  const seed = secret.slice(0, 32)
  const address = base58Encode(ed25519.getPublicKey(seed))
  if (address !== expected) throw new Error(`${name} is ${address}, expected ${expected}`)
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

const minimumBalance = bytes => BigInt(128 + bytes) * 3480n * 2n

function deployInstructions (programData) {
  const lamports = minimumBalance(PROGRAM_SPACE)
  const create = {
    programId: SYSTEM,
    accounts: [
      { pubkey: DEPLOYER, isSigner: true, isWritable: true },
      { pubkey: PROGRAM_ID, isSigner: true, isWritable: true }
    ],
    data: concat([u32(0), u64(lamports), u64(PROGRAM_SPACE), base58Decode(LOADER, 32)])
  }
  const deploy = {
    programId: LOADER,
    accounts: [
      { pubkey: DEPLOYER, isSigner: true, isWritable: true },
      { pubkey: programData, isSigner: false, isWritable: true },
      { pubkey: PROGRAM_ID, isSigner: false, isWritable: true },
      { pubkey: BUFFER, isSigner: false, isWritable: true },
      { pubkey: RENT, isSigner: false, isWritable: false },
      { pubkey: CLOCK, isSigner: false, isWritable: false },
      { pubkey: SYSTEM, isSigner: false, isWritable: false },
      { pubkey: DEPLOYER, isSigner: true, isWritable: false }
    ],
    data: concat([u32(2), u64(elf.length)])
  }
  return [create, deploy]
}

async function rpc (method, params) {
  const response = await fetch(RPC, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
  })
  const body = await response.json()
  if (body.error) throw new Error(`${method} failed: ${JSON.stringify(body.error)}`)
  return body.result
}

function writeIx (offset, bytes) {
  return {
    programId: LOADER,
    accounts: [
      { pubkey: BUFFER, isSigner: false, isWritable: true },
      { pubkey: DEPLOYER, isSigner: true, isWritable: false }
    ],
    data: concat([u32(1), u32(offset), u64(bytes.length), Uint8Array.from(bytes)])
  }
}

function signedTransaction (signers, ixList, blockhash) {
  const compiled = compileMessage(DEPLOYER, ixList, blockhash)
  const signatures = compiled.signers.map(address => {
    const signer = signers.find(key => key.address === address)
    if (!signer) throw new Error(`No signer for ${address}`)
    return ed25519.sign(compiled.message, signer.seed)
  })
  if (signatures.length > 127) throw new Error('too many signatures')
  return concat([Uint8Array.of(signatures.length), ...signatures, compiled.message])
}

function writeChunkSize () {
  const blockhash = base58Encode(new Uint8Array(32))
  for (let size = 1000; size >= 200; size -= 20) {
    const tx = signedTransaction([deployer], [writeIx(0, new Uint8Array(size))], blockhash)
    if (tx.length <= 1200) return size
  }
  throw new Error('a buffer write does not fit in one transaction')
}

function differingPieces (body, size) {
  const ranges = []
  let start = -1
  for (let index = 0; index < elf.length; index++) {
    const differs = body[index] !== elf[index]
    if (differs && start < 0) start = index
    if (!differs && start >= 0) {
      ranges.push([start, index])
      start = -1
    }
  }
  if (start >= 0) ranges.push([start, elf.length])
  const merged = []
  for (const range of ranges) {
    const previous = merged.at(-1)
    if (previous && range[0] - previous[1] <= 64) previous[1] = range[1]
    else merged.push([range[0], range[1]])
  }
  const pieces = []
  for (const [from, to] of merged) {
    for (let offset = from; offset < to; offset += size) {
      pieces.push({ offset, bytes: elf.subarray(offset, Math.min(to, offset + size)) })
    }
  }
  return pieces
}

const sleep = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))

const limited = error => {
  const text = String(error)
  return text.includes('429') || text.includes('Too many requests')
}

const mode = process.argv[2]
if (!['--prepare', '--check', '--repair', '--send', '--council'].includes(mode)) {
  console.log('Use --prepare locally, --check to compare once, --repair to patch differing bytes slowly, --send to publish once, or --council to create the upgrade council.')
  process.exit(1)
}

const deployer = loadKey('deployer-keypair.json', DEPLOYER)
const program = loadKey('program-keypair.json', PROGRAM_ID)
const programData = programDataAddress(PROGRAM_ID)
const instructions = deployInstructions(programData)

if (mode === '--prepare') {
  const blockhash = base58Encode(new Uint8Array(32))
  const compiled = compileMessage(DEPLOYER, instructions, blockhash)
  if (compiled.signers.length !== 2) throw new Error(`expected 2 signers, got ${compiled.signers.length}`)
  const chunk = writeChunkSize()
  console.log(`final deploy is one transaction, ${compiled.message.length} byte message`)
  console.log(`a repair write carries ${chunk} bytes`)
  console.log(`program data ${programData}`)
  console.log(`buffer ${BUFFER}`)
  console.log(`elf ${elf.length} bytes`)
  process.exit(0)
}

if (mode === '--council') {
  const member = loadKey('council-signer.json', COUNCIL_SIGNER)
  const derived = (() => {
    const programBytes = base58Decode(PROGRAM_ID, 32)
    for (let bump = 255; bump >= 0; bump--) {
      const hash = createHash('sha256').update(Buffer.concat([Buffer.from('upgrade-council'), Uint8Array.of(bump), programBytes, Buffer.from('ProgramDerivedAddress')])).digest()
      try {
        ed25519.ExtendedPoint.fromHex(hash)
      } catch {
        return base58Encode(hash)
      }
    }
    throw new Error('No upgrade council address')
  })()
  if (derived !== COUNCIL) throw new Error(`council address drifted to ${derived}`)
  const existing = await rpc('getAccountInfo', [COUNCIL, { encoding: 'base64', commitment: 'confirmed' }])
  if (existing.value) {
    console.log(`upgrade council already exists at ${COUNCIL}`)
    process.exit(0)
  }
  const discriminator = createHash('sha256').update('global:initialize_upgrade').digest().subarray(0, 8)
  const instruction = {
    programId: PROGRAM_ID,
    accounts: [
      { pubkey: DEPLOYER, isSigner: true, isWritable: true },
      { pubkey: COUNCIL, isSigner: false, isWritable: true },
      { pubkey: SYSTEM, isSigner: false, isWritable: false }
    ],
    data: concat([
      discriminator,
      Uint8Array.of(2),
      u32(2),
      base58Decode(deployer.address, 32),
      base58Decode(member.address, 32)
    ])
  }
  const { blockhash } = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
  const transaction = signedTransaction([deployer], [instruction], blockhash)
  const signature = await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])
  console.log(`council signature ${signature}`)
  console.log(`upgrade council ${COUNCIL}`)
  console.log(`signers ${deployer.address} ${member.address}`)
  process.exit(0)
}

async function bufferAccount () {
  const info = await rpc('getAccountInfo', [BUFFER, { encoding: 'base64', commitment: 'confirmed' }])
  const data = info.value ? Buffer.from(info.value.data[0], 'base64') : null
  if (!data || data.length !== BUFFER_HEADER + elf.length) {
    throw new Error(`buffer is ${data ? data.length : 'missing'} bytes; refusing to write it again`)
  }
  if (data[0] !== 1 || data[1] !== 0 || data[2] !== 0 || data[3] !== 0 || data[4] !== 1) {
    throw new Error('buffer header is not a loader buffer; refusing to publish it')
  }
  return data
}

function difference (data) {
  const body = data.subarray(BUFFER_HEADER)
  const onChain = createHash('sha256').update(body).digest('hex')
  const local = createHash('sha256').update(elf).digest('hex')
  return { body, onChain, local, matches: onChain === local }
}

async function repair (body) {
  const size = writeChunkSize()
  const pieces = differingPieces(body, size)
  console.log(`repairing ${pieces.length} ranges, one transaction at a time`)
  let blockhash = ''
  let fetchedAt = 0
  const freshBlockhash = async () => {
    blockhash = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value.blockhash
    fetchedAt = Date.now()
    return blockhash
  }
  for (let index = 0; index < pieces.length; index++) {
    const piece = pieces[index]
    if (!blockhash || Date.now() - fetchedAt > 20000) {
      try {
        await freshBlockhash()
      } catch (error) {
        if (limited(error)) {
          console.log(`paused after ${index} writes because the RPC rate limit returned`)
          return false
        }
        throw error
      }
    }
    const sendPiece = async () => {
      const transaction = signedTransaction([deployer], [writeIx(piece.offset, piece.bytes)], blockhash)
      await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])
    }
    try {
      await sendPiece()
    } catch (error) {
      if (limited(error)) {
        console.log(`paused after ${index} writes because the RPC rate limit returned`)
        return false
      }
      if (!String(error).includes('Blockhash not found') && !String(error).includes('BlockhashNotFound')) throw error
      try {
        await freshBlockhash()
        await sendPiece()
      } catch (retry) {
        if (limited(retry)) {
          console.log(`paused after ${index} writes because the RPC rate limit returned`)
          return false
        }
        throw retry
      }
    }
    if (index === 0 || index + 1 === pieces.length || (index + 1) % 10 === 0) {
      console.log(`wrote ${index + 1}/${pieces.length} at offset ${piece.offset}`)
    }
    await sleep(1500)
  }
  return true
}

let data = await bufferAccount()
let diff = difference(data)
if (!diff.matches && mode === '--repair') {
  const finished = await repair(diff.body)
  if (!finished) process.exit(0)
  await sleep(3000)
  data = await bufferAccount()
  diff = difference(data)
}
if (!diff.matches) {
  const pieces = differingPieces(diff.body, writeChunkSize())
  throw new Error(`buffer differs from predictions.so (${diff.onChain}); ${pieces.length} ranges still differ. Refusing to publish it.`)
}
console.log(`buffer matches predictions.so ${diff.local}`)
if (mode === '--check' || mode === '--repair') {
  if (mode === '--repair') console.log('buffer matches; publish with --send')
  process.exit(0)
}

const { blockhash } = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
const compiled = compileMessage(DEPLOYER, instructions, blockhash)
const signatures = compiled.signers.map(address => {
  const signer = [deployer, program].find(key => key.address === address)
  if (!signer) throw new Error(`No signer for ${address}`)
  return ed25519.sign(compiled.message, signer.seed)
})
if (signatures.length !== 2) throw new Error('the final deploy must have two signatures')
const transaction = concat([Uint8Array.of(signatures.length), ...signatures, compiled.message])
const signature = await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])
console.log(`deploy signature ${signature}`)
console.log(`program ${PROGRAM_ID}`)
console.log(`program data ${programData}`)
