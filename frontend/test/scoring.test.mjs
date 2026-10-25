import assert from 'node:assert/strict'
import test from 'node:test'
import { combinationCount, expandCombinations } from '../src/utils/coupons.js'
import { correctCounts, couponPayout, couponScore } from '../src/services/program.js'

const one = 0b001
const draw = 0b010
const two = 0b100

test('combination count is the product of the picks and does not build the rows', () => {
  assert.equal(combinationCount([['1', 'X'], ['2'], ['1', '2']]), 4)
  assert.equal(combinationCount([[]]), 0)
  assert.deepEqual(expandCombinations([['1', 'X'], ['2']]), ['12', 'X2'])
})

test('a void result counts as a hit and the payout is one row times its rows', () => {
  const picks = [one | draw, draw, two]
  assert.deepEqual(correctCounts(picks, '1X2').slice(2), [1, 1])
  assert.deepEqual(correctCounts([one, one, two], '1C2'), [0, 0, 0, 1])

  const event = {
    status: 'resolved',
    results: '1X2',
    tiers: [{ correct: 3, percent: 50 }, { correct: 2, percent: 50 }],
    perWinner: [0n, 1000n, 2000n]
  }
  const score = couponScore({ revealed: true, picks, gameCount: 3 }, event)
  assert.equal(score.best, 3)
  assert.deepEqual(score.lines, [
    { correct: 3, rows: 1, each: 2000n },
    { correct: 2, rows: 1, each: 1000n }
  ])
  assert.equal(couponPayout({ revealed: true, picks, gameCount: 3 }, event), 3000n)
  assert.equal(couponScore({ revealed: false, picks, gameCount: 3 }, event), null)
})

test('a stored session needs a wallet and a future expiry, not a token', async () => {
  const saved = new Map()
  globalThis.localStorage = {
    getItem: key => saved.get(key) ?? null,
    setItem: (key, value) => saved.set(key, String(value)),
    removeItem: key => saved.delete(key)
  }
  const session = await import('../src/services/session.js')
  session.saveSession({ token: 'secret', expiresAt: '2999-01-01T00:00:00Z' })
  assert.equal(session.loadSession(), null)
  session.saveSession({ wallet: 'WalletA', expiresAt: '2999-01-01T00:00:00Z' })
  assert.equal(session.loadSession().wallet, 'WalletA')
  session.saveSession({ wallet: 'WalletA', expiresAt: '2000-01-01T00:00:00Z' })
  assert.equal(session.loadSession(), null)
})
