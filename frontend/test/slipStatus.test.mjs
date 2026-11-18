import assert from 'node:assert/strict'
import test from 'node:test'
import { publishedSlipPatch, slipFromArchive } from '../src/services/slipStatus.js'

const wallet = 'Owner111'
const coupon = { slip: true, index: 3, revealed: false }

test('a keeper reveal marks the slip revealed without a browser signature', () => {
  const patch = publishedSlipPatch(coupon, {
    reveals: [{ index: 3, owner: wallet }],
    claims: [],
    refunds: []
  }, wallet)
  assert.deepEqual(patch, { revealed: true })
})

test('another wallet reveal is ignored', () => {
  const patch = publishedSlipPatch(coupon, {
    reveals: [{ index: 3, owner: 'SomeoneElse' }]
  }, wallet)
  assert.deepEqual(patch, {})
})

test('an already recorded reveal is left unchanged', () => {
  const patch = publishedSlipPatch({ ...coupon, revealed: true }, {
    reveals: [{ index: 3, owner: wallet }],
    claims: [{ index: 3, owner: wallet }]
  }, wallet)
  assert.deepEqual(patch, { claimed: true })
})

test('a claim log without an owner still closes that coupon number', () => {
  const patch = publishedSlipPatch(coupon, {
    claims: [{ index: 3 }]
  }, wallet)
  assert.deepEqual(patch, { claimed: true })
})

test('a slip from another device is listed from the archive', () => {
  const built = slipFromArchive({
    wallet,
    event: { id: 'event-1', title: 'Matchday', admin: 'Admin', nonce: 4 },
    address: 'EventAddress',
    row: { owner: wallet, index: 2, combinations: 18, commitment: 'ab', signature: 'sig' },
    reveal: { owner: wallet, index: 2, picks: '0102' },
    claimed: false,
    refunded: false
  })
  assert.equal(built.address, 'slip:EventAddress:2')
  assert.equal(built.eventTitle, 'Matchday')
  assert.equal(built.revealed, true)
  assert.equal(built.picksHex, '0102')
})

test('someone else\'s slip is not adopted', () => {
  assert.equal(slipFromArchive({
    wallet,
    event: { id: 'event-1', admin: 'Admin', nonce: 4 },
    address: 'EventAddress',
    row: { owner: 'SomeoneElse', index: 2, combinations: 1, commitment: 'ab' }
  }), null)
})

test('the coupon number is read from the confirmed transaction', async () => {
  const { slipIndexFromLogs } = await import('../src/services/slipStatus.js')
  const logs = [
    'Program AaXA invoke [1]',
    'Program log: Instruction: PlaceCanopySlip',
    'Program log: slip-v1:EventAddress:41:SomeoneElse:1:ab',
    'Program log: slip-v1:EventAddress:42:Owner111:3:cd'
  ]
  assert.equal(slipIndexFromLogs(logs, 'EventAddress', wallet, 'cd'), 42)
  assert.equal(slipIndexFromLogs(logs, 'EventAddress', wallet, 'ab'), null)
  assert.equal(slipIndexFromLogs(logs, 'OtherEvent', wallet, 'cd'), null)
  assert.equal(slipIndexFromLogs(undefined, 'EventAddress', wallet, 'cd'), null)
})

test('a slip saved under its commitment has no number yet', async () => {
  const { pendingSlipAddress, slipNumbered } = await import('../src/services/slipStatus.js')
  const commitment = '07'.repeat(32)
  assert.equal(slipNumbered({ slip: true, index: 7, address: pendingSlipAddress('EventAddress', commitment) }), false)
  assert.equal(slipNumbered({ slip: true, index: 7, address: 'slip:EventAddress:7' }), true)
  assert.equal(slipNumbered({ slip: true, index: 7, address: 'slip:EventAddress:17' }), false)
  assert.equal(slipNumbered({ index: 7, address: 'CouponAccount' }), true)
})
