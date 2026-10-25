// Creates the monthly draw account. The admin wallet pays its rent once.
//
//   node scripts/init-lottery.mjs

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Encode } from '../src/services/solana.js'
import { compileMessage, initLotteryIx, lotteryPda } from '../src/services/program.js'

const RPC = process.env.SOLANA_RPC_URL || 'https://api.devnet.solana.com'
const ADMIN = '4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../programs/predictions')

function loadAdmin () {
  const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(root, 'keys', 'admin-keypair.json'), 'utf8')))
  const seed = secret.slice(0, 32)
  const address = base58Encode(ed25519.getPublicKey(seed))
  if (address !== ADMIN) throw new Error(`admin key is ${address}`)
  return { seed, address }
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

const admin = loadAdmin()
const lottery = lotteryPda(admin.address)
const existing = await rpc('getAccountInfo', [lottery, { encoding: 'base64' }])
if (existing.value) {
  console.log(`monthly draw already exists at ${lottery}`)
  process.exit(0)
}

const { value } = await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])
const compiled = compileMessage(admin.address, [initLotteryIx(admin.address)], value.blockhash)
const signature = ed25519.sign(compiled.message, admin.seed)
const tx = Buffer.concat([Buffer.from([1]), Buffer.from(signature), Buffer.from(compiled.message)])
const sent = await rpc('sendTransaction', [tx.toString('base64'), { encoding: 'base64', skipPreflight: false, preflightCommitment: 'confirmed' }])
console.log(`monthly draw ${lottery}`)
console.log(sent)
