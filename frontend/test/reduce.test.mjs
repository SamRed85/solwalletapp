import assert from 'node:assert/strict'
import test from 'node:test'
import { colorReduction, colorView, keptCounts, missedColorGames, ReductionError } from '../src/utils/reduce.js'

const full = count => Array.from({ length: count }, () => ['1', 'X', '2'])
const OUTCOMES = ['1', 'X', '2']

function rowsOf (system) {
  const games = system.map(game => [...game])
  const rows = []
  function walk (index, row) {
    if (index === games.length) {
      rows.push(row)
      return
    }
    for (const outcome of games[index]) walk(index + 1, row + outcome)
  }
  walk(0, '')
  return rows
}

function survivors (frame, colors) {
  return rowsOf(frame.map(game => game.join(''))).filter(row => colors.every(color => {
    const hits = color.signs.filter(sign => row[sign.game] === sign.outcome).length
    return hits >= color.min && hits <= color.max
  }))
}

test('only the color that lands outside its range marks its games', () => {
  const purple = { signs: 0b001001n, min: 2, max: 2 }
  const yellow = { signs: 0b001n << 6n, min: 0, max: 0 }
  const missed = missedColorGames([purple, yellow], '111')
  assert.deepEqual([...missed], [2])
  assert.equal(missedColorGames([purple], '111').size, 0)
})

test('six yellow favorites with min 3 and max 5 are one coupon with the kept rows', () => {
  const frame = full(8)
  const signs = [0, 1, 2, 3, 4, 5].map(game => ({ game, outcome: '1' }))
  const reduced = colorReduction(frame, [{ signs, min: 3, max: 5 }])
  assert.equal(reduced.rows, 20 * (2 ** 3) * 9 + 15 * (2 ** 2) * 9 + 6 * (2 ** 1) * 9)
  assert.equal(reduced.rows, survivors(frame, [{ signs, min: 3, max: 5 }]).length)
  assert.equal(reduced.frameRows, 3 ** 8)
  assert.deepEqual(reduced.colors, [{ signs: 0b001001001001001001n, min: 3, max: 5 }])
})

test('random colors keep exactly the rows inside every range', () => {
  let seed = 11
  const next = limit => {
    seed = (seed * 1103515245 + 12345) % 2147483648
    return seed % limit
  }
  for (let trial = 0; trial < 150; trial += 1) {
    const games = 1 + next(6)
    const frame = Array.from({ length: games }, () => {
      const picked = OUTCOMES.filter(() => next(2))
      return picked.length ? picked : [OUTCOMES[next(3)]]
    })
    const colors = Array.from({ length: next(4) }, () => {
      const signs = []
      frame.forEach((outcomes, game) => outcomes.forEach(outcome => { if (next(2)) signs.push({ game, outcome }) }))
      const min = next(games + 1)
      return { signs, min, max: min + next(games + 1 - min) }
    })
    const want = survivors(frame, colors).length
    let got
    try {
      got = colorReduction(frame, colors).rows
    } catch (error) {
      assert.ok(error instanceof ReductionError)
      if (/no rows/.test(error.message)) assert.equal(want, 0)
      continue
    }
    assert.equal(got, want, JSON.stringify({ frame, colors }))
  }
})

test('kept rows by score match every row', () => {
  const picks = [0b111, 0b011, 0b110, 0b111]
  const colors = [{ signs: (1n << 0n) | (1n << 3n) | (1n << 9n), min: 1, max: 2 }]
  const results = '1X2C'
  const want = new Array(5).fill(0)
  for (const row of rowsOf(['1X2', '1X', 'X2', '1X2'])) {
    const hits = (row[0] === '1') + (row[1] === '1') + (row[3] === '1')
    if (hits < 1 || hits > 2) continue
    let score = 0
    for (let game = 0; game < 4; game += 1) if (results[game] === 'C' || results[game] === row[game]) score += 1
    want[score] += 1
  }
  assert.deepEqual(keptCounts(picks, colors, results), want)
  assert.equal(keptCounts(picks, colors)[0], want.reduce((total, rows) => total + rows, 0))
})

test('every outcome on 13 games is one coupon once the colors bring it under the limit', () => {
  const frame = full(13)
  const home = frame.map((_, game) => ({ game, outcome: '1' }))
  assert.throws(
    () => colorReduction(frame, []),
    error => error instanceof ReductionError && error.rows === 3 ** 13 && /still kept/.test(error.message)
  )
  const over = frame.map((_, game) => ({ game, outcome: '1' }))
  assert.throws(
    () => colorReduction(frame, [{ signs: over, min: 0, max: 9 }]),
    error => error instanceof ReductionError && error.rows > 44000 && error.frameRows === 3 ** 13
  )
  assert.equal(colorReduction(frame, [{ signs: home, min: 13, max: 13 }]).rows, 1)
  assert.equal(colorReduction(frame, [{ signs: home, min: 0, max: 0 }]).rows, 2 ** 13)
  const away = frame.map((_, game) => ({ game, outcome: '2' }))
  const both = frame.map((_, game) => [{ game, outcome: '1' }, { game, outcome: '2' }]).flat()
  assert.equal(colorReduction(frame, [{ signs: both, min: 13, max: 13 }]).rows, 2 ** 13)
  assert.equal(colorReduction(frame, [
    { signs: home, min: 3, max: 3 },
    { signs: frame.map((_, game) => ({ game, outcome: 'X' })), min: 3, max: 3 },
    { signs: away.slice(0, 4), min: 0, max: 3 }
  ]).rows, 32640)
})

test('colors past the program state cap are refused', () => {
  const frame = full(13)
  const home = frame.map((_, game) => ({ game, outcome: '1' }))
  const draw = frame.map((_, game) => ({ game, outcome: 'X' }))
  assert.throws(() => colorReduction(frame, [{ signs: home, min: 1, max: 8 }, { signs: draw, min: 1, max: 8 }]), /too detailed/)
})

test('a sign outside the frame cannot be hit', () => {
  const frame = [['1'], ['X'], ['2']]
  assert.throws(
    () => colorReduction(frame, [{ signs: [{ game: 0, outcome: '2' }], min: 1, max: 1 }]),
    ReductionError
  )
})

test('a saved color bitset names the same signs the player marked', () => {
  const view = colorView([{ signs: '73', min: 1, max: 2 }, { signs: '272', min: 0, max: 1 }])
  assert.deepEqual(view[0], {
    id: 'purple', label: 'Purple', min: 1, max: 2,
    marks: [{ game: 0, outcome: '1' }, { game: 1, outcome: '1' }, { game: 2, outcome: '1' }]
  })
  assert.equal(view[1].id, 'yellow')
  assert.deepEqual(view[1].marks, [{ game: 1, outcome: 'X' }, { game: 2, outcome: '2' }])
})

test('a coupon reduced with yellow only stays yellow', () => {
  const frame = full(3)
  const reduced = colorReduction(frame, [{ id: 'yellow', signs: [{ game: 0, outcome: '1' }, { game: 1, outcome: '1' }], min: 1, max: 1 }])
  assert.equal(reduced.colors[0].id, 'yellow')
  const view = colorView(reduced.colors.map(color => ({ ...color, signs: color.signs.toString() })))
  assert.equal(view[0].id, 'yellow')
  assert.equal(view[0].label, 'Yellow')
})

test('a color that keeps every row is left off the coupon', () => {
  const reduced = colorReduction([['1', 'X'], ['2'], ['1']], [{ signs: [{ game: 0, outcome: '1' }], min: 0, max: 1 }])
  assert.deepEqual(reduced, { rows: 2, frameRows: 2, system: ['1X', '2', '1'], colors: [] })
})
