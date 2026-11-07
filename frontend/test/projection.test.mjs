import assert from 'node:assert/strict'
import test from 'node:test'
import { combinedScore, kickoffLabel, matchClock, projectCoupon, showStandingPayout, standingResults } from '../src/services/projection.js'

const projection = {
  complete: true,
  maximum: true,
  games: [{ projected: '1', linked: true }, { projected: 'X', linked: true }],
  tiers: [
    { correct: 2, potLamports: 700, perWinnerLamports: 700 },
    { correct: 1, potLamports: 250, perWinnerLamports: 250 }
  ]
}

test('a live score projects the maximum while other coupons are hidden', () => {
  const view = projectCoupon(['1', 'X'], projection)
  assert.equal(view.maximum, true)
  assert.equal(view.best, 2)
  assert.equal(view.lines.length, 1)
  assert.equal(view.total, 700n)
})

test('several rows of this coupon share one tier instead of each taking the whole pot', () => {
  const view = projectCoupon(['1X', 'X2'], projection)
  const lower = view.lines.find(line => line.correct === 1)
  assert.equal(lower.rows, 2)
  assert.equal(lower.each, 125n)
})

test('every row in a tier takes the same share, however many rows this coupon has', () => {
  const shared = {
    ...projection,
    maximum: false,
    tiers: [
      { correct: 2, potLamports: 1500, perWinnerLamports: 100 },
      { correct: 1, potLamports: 250, perWinnerLamports: 10 }
    ]
  }
  const one = projectCoupon(['1', 'X'], shared)
  const wider = projectCoupon(['1X', 'X'], shared)
  const twoCorrect = wider.lines.find(line => line.correct === 2)
  assert.equal(one.lines[0].each, 100n)
  assert.equal(twoCorrect.each, 100n)
  assert.equal(one.total, 100n)
  assert.equal(wider.total, 110n)
})

test('after the reveal window the shared payout is used', () => {
  const shared = { ...projection, maximum: false, tiers: projection.tiers.map(tier => ({ ...tier, perWinnerLamports: tier.correct === 2 ? 350 : 0 })) }
  const view = projectCoupon(['1', 'X'], shared)
  assert.equal(view.maximum, false)
  assert.equal(view.lines[0].each, 350n)
  assert.equal(view.lines.length, 1)
})

test('a match today is labelled with the kickoff time', () => {
  const now = new Date(2026, 9, 25, 18, 0)
  const text = kickoffLabel(new Date(2026, 9, 25, 20, 45), now)
  assert.match(text, /^Today /)
  assert.match(text, /45/)
  assert.match(kickoffLabel(new Date(2026, 9, 26, 20, 45), now), /26/)
})

test('a started match shows the minute, and added time keeps the plus', () => {
  assert.equal(matchClock({ status: '1H', elapsed: 34 }), "34'")
  assert.equal(matchClock({ status: '2H', elapsed: 90, extra: 2 }), "90+'")
  assert.equal(matchClock({ status: 'FT', elapsed: 90 }), 'FT')
  assert.equal(matchClock({ status: 'HT', elapsed: 45 }), 'HT')
})

test('a match that has not started stands as X beside the live score', () => {
  assert.equal(standingResults({
    games: [{ projected: '1', homeGoals: 2, awayGoals: 1 }, { projected: 'X', status: 'NS' }]
  }), '1X')
  assert.equal(standingResults({ games: [{ projected: '1' }, { projected: '' }] }), '')
})

test('rows from every coupon on an event are added together', () => {
  const tiers = [
    { correct: 2, potLamports: 700, perWinnerLamports: 100 },
    { correct: 1, potLamports: 250, perWinnerLamports: 10 }
  ]
  const view = combinedScore([['1', 'X'], ['1X', 'X']], '1X', tiers, false)
  assert.equal(view.best, 2)
  assert.equal(view.lines.find(line => line.correct === 2).rows, 2)
  assert.equal(view.lines.find(line => line.correct === 2).each, 100n)
  assert.equal(view.lines.find(line => line.correct === 1).rows, 1)
  const alone = projectCoupon(['1', 'X'], { ...projection, maximum: false, tiers })
  const same = combinedScore([['1', 'X']], '1X', tiers, false)
  assert.equal(same.lines[0].each, alone.lines[0].each)
  assert.equal(same.lines[0].rows, alone.lines[0].rows)
})

test('the standing payout shows while matches are still being played', () => {
  const live = { revealClosed: false, games: [{ projected: '1', final: false }, { projected: 'X', final: true }] }
  const finished = { revealClosed: false, games: [{ projected: '1', final: true }, { projected: 'X', final: true }] }
  const beforeKickoff = { revealClosed: false, games: [{ projected: 'X', final: false }, { projected: 'X', final: false }] }
  const firstUnderway = { revealClosed: false, games: [{ projected: '1', final: false, status: '1H', elapsed: 12 }, { projected: 'X', final: false, status: 'NS' }] }
  assert.equal(showStandingPayout(live), true)
  assert.equal(showStandingPayout(finished), true)
  assert.equal(showStandingPayout(firstUnderway), true)
  assert.equal(showStandingPayout(beforeKickoff), false)
  assert.equal(showStandingPayout(null), false)
})

test('a match without a score has no payout', () => {
  assert.equal(projectCoupon(['1', 'X'], { ...projection, complete: false }), null)
  assert.equal(projectCoupon(['1', ''], projection), null)
})
