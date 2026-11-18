import assert from 'node:assert/strict'
import test from 'node:test'
import { totalsSignatureLocks } from '../src/services/program.js'

const admin = 'admin'
const cosigner = 'cosigner'

test('the first signature does not lock', () => {
  assert.equal(totalsSignatureLocks({
    wallet: admin, members: [admin, cosigner], threshold: 2, approvals: 0, sameList: true
  }), false)
})

test('a browser signature locks once the other signer has approved the same list', () => {
  assert.equal(totalsSignatureLocks({
    wallet: admin, members: [admin, cosigner], threshold: 2, approvals: 2, sameList: true
  }), true)
})

test('a different list does not lock on top of the old approvals', () => {
  assert.equal(totalsSignatureLocks({
    wallet: admin, members: [admin, cosigner], threshold: 2, approvals: 2, sameList: false
  }), false)
})
