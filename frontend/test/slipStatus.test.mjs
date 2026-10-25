import assert from 'node:assert/strict'
import test from 'node:test'
import { publishedSlipPatch } from '../src/services/slipStatus.js'

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
