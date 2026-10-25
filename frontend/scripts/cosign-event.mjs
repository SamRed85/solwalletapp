// Sign the second approval for a vault event with the council key on disk.
// The backend names this key as RESULT_COSIGNER, so it is not a Phantom account.
//   node scripts/cosign-event.mjs <event> cancel
//   node scripts/cosign-event.mjs <event> resolve 1X21C11

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ed25519 } from '@noble/curves/ed25519'
import { base58Encode } from '../src/services/solana.js'
import {
  approveCancelIx, approveResolutionIx, compileMessage, decodeEvent, decodeSigners, RESULT_COSIGNER, signersPda
} from '../src/services/program.js'

const url = process.env.SOLANA_RPC_URL || 'https://api.devnet.solana.com'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../programs/predictions/keys')
const [event, action, results] = process.argv.slice(2)

if (!event || !['cancel', 'resolve'].includes(action) || (action === 'resolve' && !results)) {
  console.error('Usage: node scripts/cosign-event.mjs <event> cancel | resolve <results>')
  process.exit(1)
}

async function rpc (method, params) {
  for (let attempt = 0; attempt < 8; attempt++) {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
    })
    if (response.status === 429) {
      await new Promise(resolve => setTimeout(resolve, 3000 * (attempt + 1)))
      continue
    }
    const body = await response.json()
    if (body.error) throw new Error(`${method} failed: ${JSON.stringify(body.error)}`)
    return body.result
  }
  throw new Error(`${method} was rate limited`)
}

async function account (address) {
  const info = await rpc('getAccountInfo', [address, { encoding: 'base64', commitment: 'confirmed' }])
  if (!info.value) throw new Error(`${address} does not exist`)
  return Buffer.from(info.value.data[0], 'base64')
}

const secret = Uint8Array.from(JSON.parse(readFileSync(resolve(root, 'council-signer.json'), 'utf8')))
const seed = secret.slice(0, 32)
const address = base58Encode(ed25519.getPublicKey(seed))
if (address !== RESULT_COSIGNER) throw new Error(`council-signer.json is ${address}, expected ${RESULT_COSIGNER}`)

const state = decodeEvent(await account(event))
const { members } = decodeSigners(await account(signersPda(event)))
if (!members.includes(address)) throw new Error(`${address} is not a signer of this event`)
console.log('before', { status: state.status, proposalKind: state.proposalKind, approvals: state.approvals })

const instruction = action === 'cancel'
  ? approveCancelIx({ signer: address, admin: state.admin, event })
  : approveResolutionIx({ signer: address, admin: state.admin, event, results })
const { blockhash } = (await rpc('getLatestBlockhash', [{ commitment: 'confirmed' }])).value
const compiled = compileMessage(address, [instruction], blockhash)
const signature = ed25519.sign(compiled.message, seed)
const transaction = new Uint8Array(1 + 64 + compiled.message.length)
transaction.set([1], 0)
transaction.set(signature, 1)
transaction.set(compiled.message, 65)
const sent = await rpc('sendTransaction', [Buffer.from(transaction).toString('base64'), { encoding: 'base64', preflightCommitment: 'confirmed' }])

for (let attempt = 0; attempt < 40; attempt++) {
  const status = (await rpc('getSignatureStatuses', [[sent], { searchTransactionHistory: true }])).value[0]
  if (status?.err) throw new Error(`transaction ${sent} failed: ${JSON.stringify(status.err)}`)
  if (['confirmed', 'finalized'].includes(status?.confirmationStatus)) break
  await new Promise(resolve => setTimeout(resolve, 1000))
}
const after = decodeEvent(await account(event))
console.log('after', { status: after.status, proposalKind: after.proposalKind, approvals: after.approvals, signature: sent })
