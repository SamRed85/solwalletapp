import { createHash } from 'node:crypto'

const [signature, eventId, payer, treasury, lamportsText, systemText] = process.argv.slice(2)
if (!signature || !eventId || !payer || !treasury || !lamportsText || !systemText) {
  console.error('Usage: node scripts/verify-devnet-payment.mjs <signature> <event-id> <payer> <treasury> <lamports> <game-sets-comma-separated>')
  process.exit(2)
}

const lamports = Number(lamportsText)
if (!Number.isSafeInteger(lamports) || lamports <= 0) {
  throw new Error('lamports must be a positive safe integer')
}

const system = systemText.split(',')
if (system.some(picks => !/^(?:1|X|2|1X|12|X2|1X2)$/.test(picks))) {
  throw new Error('each game set must be canonical and contain only 1, X, 2')
}

const canonical = `${eventId}\n${system.join(',')}`
const expectedMemo = `sol-entry-v1:${createHash('sha256').update(canonical).digest('hex')}`
const response = await fetch('https://api.devnet.solana.com', {
  method: 'POST',
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify({
    jsonrpc: '2.0',
    id: 1,
    method: 'getTransaction',
    params: [signature, {
      encoding: 'jsonParsed',
      commitment: 'finalized',
      maxSupportedTransactionVersion: 0
    }]
  })
})

if (response.status === 429) throw new Error('devnet RPC rate limited the request; retry shortly')
const rpc = await response.json()
if (rpc.error) throw new Error(`devnet RPC: ${rpc.error.message}`)
if (!rpc.result) throw new Error('transaction was not found at finalized commitment')
if (rpc.result.meta?.err) throw new Error(`transaction failed: ${JSON.stringify(rpc.result.meta.err)}`)

let paid = 0
const memos = []
for (const instruction of rpc.result.transaction.message.instructions) {
  if (instruction.program === 'system' &&
      instruction.parsed?.type === 'transfer' &&
      instruction.parsed.info.source === payer &&
      instruction.parsed.info.destination === treasury) {
    paid += Number(instruction.parsed.info.lamports)
  }
  if (instruction.program === 'spl-memo' && typeof instruction.parsed === 'string') {
    memos.push(instruction.parsed)
  }
}

if (paid !== lamports) {
  throw new Error(`payment mismatch: expected ${lamports}, found ${paid}`)
}
if (memos.length !== 1 || memos[0] !== expectedMemo) {
  throw new Error(`memo mismatch: expected ${expectedMemo}, found ${JSON.stringify(memos)}`)
}

console.log(JSON.stringify({
  verified: true,
  cluster: 'devnet',
  signature,
  slot: rpc.result.slot,
  blockTime: new Date(rpc.result.blockTime * 1000).toISOString(),
  payer,
  treasury,
  lamports,
  memo: expectedMemo
}, null, 2))
