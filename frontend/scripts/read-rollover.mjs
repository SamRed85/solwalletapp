import { readFileSync } from 'node:fs'
import { decodeEvent } from '../src/services/program.js'

const env = readFileSync(new URL('../../.env', import.meta.url), 'utf8')
const rpc = env.split(/\r?\n/).find(line => line.startsWith('SOLANA_RPC_URL=')).slice('SOLANA_RPC_URL='.length).trim().replace(/^["']|["']$/g, '')
const address = process.argv[2] || '5SwFUSeWP2gLzRuYWtpENpv32PobtDGqiWEg9rMjV3t3'
const res = await fetch(rpc, {
  method: 'POST',
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'getAccountInfo', params: [address, { encoding: 'base64', commitment: 'confirmed' }] })
})
const json = await res.json()
if (!json.result?.value) {
  console.log('missing account')
  process.exit(1)
}
const data = Buffer.from(json.result.value.data[0], 'base64')
const ev = decodeEvent(data)
const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
console.log(JSON.stringify({
  len: data.length,
  rolloverIn: ev.rolloverIn.toString(),
  rolloverOut: view.getBigUint64(166, true).toString(),
  paid: ev.paid.toString(),
  couponCount: ev.couponCount,
  status: ev.status,
  minRows: ev.minRows,
  maxCoupons: ev.maxCoupons
}))
