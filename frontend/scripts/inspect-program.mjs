import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Decode, base58Encode } from '../src/services/solana.js'
import { findPda, PROGRAM_ID } from '../src/services/program.js'

const RPC = 'https://api.devnet.solana.com'
const LOADER = 'BPFLoaderUpgradeab1e11111111111111111111111'
const DEPLOYER = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'

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
  throw new Error('no program data address')
}

const rpc = async (method, params) => {
  const response = await fetch(RPC, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
  })
  const body = await response.json()
  if (body.error) throw new Error(JSON.stringify(body.error))
  return body.result
}

const data = programDataAddress(PROGRAM_ID)
const info = await rpc('getAccountInfo', [data, { encoding: 'base64', commitment: 'confirmed' }])
const raw = Buffer.from(info.value.data[0], 'base64')
const prog = await rpc('getAccountInfo', [PROGRAM_ID, { encoding: 'base64', commitment: 'confirmed' }])
const elf = readFileSync(new URL('../../programs/predictions/target/deploy/predictions.so', import.meta.url))
console.log(JSON.stringify({
  program: PROGRAM_ID,
  executable: prog.value.executable,
  data,
  bytes: raw.length,
  state: raw.readUInt32LE(0),
  option: raw[12],
  authority: raw[12] === 1 ? base58Encode(raw.subarray(13, 45)) : null,
  hash: createHash('sha256').update(raw.subarray(45, 45 + elf.length)).digest('hex'),
  local: createHash('sha256').update(elf).digest('hex'),
  council: findPda([new TextEncoder().encode('upgrade-council')]),
  deployer: (await rpc('getBalance', [DEPLOYER, { commitment: 'confirmed' }])).value
}, null, 2))
