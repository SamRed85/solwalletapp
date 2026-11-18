import assert from 'node:assert/strict'
import test from 'node:test'
import { blankHold, shownLimit, stepHold } from '../src/utils/colorMarks.js'
import { colorPaint, colorReduction, colorView, keptCounts, missedColorGames, ReductionError } from '../src/utils/reduce.js'

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

test('a pick shows every color that was put on it', () => {
  assert.equal(colorPaint([]), undefined)
  assert.equal(colorPaint(['yellow']).background, '#F9A825')
  assert.equal(colorPaint(['yellow']).color, '#1a1a1a')
  const two = colorPaint(['purple', 'red'])
  assert.match(two.background, /#6A1B9A 0% 50%/)
  assert.match(two.background, /#C62828 50% 100%/)
  assert.match(two.background, /border-box$/)
  const three = colorPaint(['purple', 'yellow', 'green'])
  assert.match(three.background, /#6A1B9A 0% 33\.333/)
  assert.match(three.background, /#F9A825 33\.333/)
  assert.match(three.background, /#2E7D32 66\.666/)
})

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
      got = error.rows
    }
    assert.equal(got, want, JSON.stringify({ frame, colors }))
  }
})

test('every overlap of the four colors keeps the rows that pass every range', () => {
  const frame = full(3)
  const frameRows = 3 ** 3
  const ids = ['purple', 'yellow', 'red', 'green']
  let checked = 0
  function expectRows (colors) {
    const want = survivors(frame, colors).length
    let got
    try {
      got = colorReduction(frame, colors).rows
    } catch (error) {
      assert.ok(error instanceof ReductionError, error.message)
      got = error.rows
    }
    assert.equal(got, want, JSON.stringify(colors))
    if (want !== frameRows) assert.notEqual(got, frameRows)
    checked += 1
  }

  // Each color marks any subset of the three games, including a second outcome on the first game.
  for (let packed = 0; packed < 16 ** 4; packed += 1) {
    const colors = ids.map((id, index) => {
      const mask = Math.floor(packed / (16 ** index)) % 16
      const signs = []
      for (let game = 0; game < 3; game += 1) {
        if (mask & (1 << game)) signs.push({ game, outcome: '1' })
      }
      if (mask & 8) signs.push({ game: 0, outcome: 'X' })
      const reach = new Set(signs.map(sign => sign.game)).size
      const min = Math.min(index === 3 ? 1 : 0, reach)
      const max = index === 1 ? 0 : reach + (index === 2 ? 5 : 0)
      return { id, signs, min, max: Math.max(max, min) }
    })
    expectRows(colors)
  }

  const overlapping = [
    { id: 'purple', signs: [0, 1, 2].map(game => ({ game, outcome: '1' })) },
    { id: 'yellow', signs: [0, 1, 2].map(game => ({ game, outcome: 'X' })) },
    { id: 'red', signs: [0, 1, 2].map(game => ({ game, outcome: '2' })) },
    { id: 'green', signs: [{ game: 0, outcome: '1' }, { game: 0, outcome: 'X' }, { game: 1, outcome: '2' }, { game: 2, outcome: '1' }] }
  ]
  const limits = []
  for (let min = 0; min <= 4; min += 1) {
    for (let max = min; max <= 4; max += 1) limits.push([min, max])
  }
  limits.push([0, 20])
  function walk (index, chosen) {
    if (index === overlapping.length) {
      expectRows(overlapping.map((color, i) => ({ ...color, min: chosen[i][0], max: chosen[i][1] })))
      return
    }
    for (const pair of limits) walk(index + 1, [...chosen, pair])
  }
  walk(0, [])
  assert.equal(checked, 16 ** 4 + limits.length ** 4)
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

test('a complex reduction keeps its row count when it cannot be one coupon', () => {
  const frame = full(6)
  const colors = [0, 1, 2, 0].map(outcome => ({
    signs: frame.map((_, game) => ({ game, outcome: ['1', 'X', '2'][outcome] })),
    min: 1,
    max: 3
  }))
  colors[3] = { signs: [{ game: 0, outcome: '1' }, { game: 1, outcome: 'X' }, { game: 2, outcome: '2' }, { game: 3, outcome: '1' }], min: 1, max: 3 }
  const want = survivors(frame, colors).length
  assert.ok(want > 0 && want < 3 ** 6)
  try {
    colorReduction(frame, colors)
    assert.fail('the state table is over 64')
  } catch (error) {
    assert.ok(error instanceof ReductionError)
    assert.match(error.message, /too detailed/)
    assert.equal(error.rows, want)
    assert.notEqual(error.rows, 3 ** 6)
  }
})

test('a max above the number of games counts the rows it can actually reach', () => {
  const frame = full(8)
  const signs = frame.map((_, game) => ({ game, outcome: '1' }))
  const reduced = colorReduction(frame, [{ signs, min: 1, max: 20 }])
  assert.equal(reduced.rows, 3 ** 8 - 2 ** 8)
  assert.equal(reduced.colors[0].max, 8)
  assert.equal(reduced.rows, survivors(frame, [{ signs, min: 1, max: 20 }]).length)
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

function signsOf (marks) {
  return marks.map(key => {
    const [game, outcome] = key.split(':')
    return { game: Number(game), outcome }
  })
}

function reachOf (marks) {
  return new Set(marks.map(key => key.split(':')[0])).size
}

test('a lowered maximum stays put when signs come off and go back on', () => {
  const frame = full(6)
  const keys = frame.map((_, game) => `${game}:1`)
  let marks = []
  let hold = blankHold()
  for (const key of keys) marks = [...marks, key]
  assert.deepEqual(shownLimit(hold, reachOf(marks)), { min: 0, max: 6 })
  for (let step = 0; step < 4; step += 1) hold = stepHold(hold, reachOf(marks), 'max', -1)
  assert.equal(shownLimit(hold, reachOf(marks)).max, 2)
  marks = marks.slice(0, 2)
  assert.equal(shownLimit(hold, reachOf(marks)).max, 2)
  marks = [...marks, keys[2], '0:X']
  assert.equal(reachOf(marks), 3)
  assert.equal(shownLimit(hold, reachOf(marks)).max, 2)
  marks = [...keys, '0:X']
  const shown = shownLimit(hold, reachOf(marks))
  assert.deepEqual(shown, { min: 0, max: 2 })
  const colors = [{ signs: signsOf(marks), ...shown }]
  const reduced = colorReduction(frame, colors)
  assert.equal(reduced.rows, survivors(frame, colors).length)
  assert.ok(reduced.rows < 3 ** 6)
})

test('a minimum comes back when its games are marked again', () => {
  let hold = blankHold()
  let marks = ['0:1', '1:1', '2:1', '3:1']
  for (let step = 0; step < 3; step += 1) hold = stepHold(hold, reachOf(marks), 'min', 1)
  assert.deepEqual(shownLimit(hold, reachOf(marks)), { min: 3, max: 4 })
  marks = ['0:1']
  assert.deepEqual(shownLimit(hold, reachOf(marks)), { min: 1, max: 1 })
  marks = ['0:1', '1:1', '2:1', '3:1']
  assert.deepEqual(shownLimit(hold, reachOf(marks)), { min: 3, max: 4 })
})

test('two signs on one game do not raise the maximum a row can hit', () => {
  const hold = stepHold(blankHold(), 1, 'max', -1)
  assert.deepEqual(shownLimit(hold, 1), { min: 0, max: 0 })
  assert.deepEqual(shownLimit(hold, 2), { min: 0, max: 0 })
  const frame = full(2)
  const colors = [{ signs: [{ game: 0, outcome: '1' }, { game: 0, outcome: 'X' }], min: 0, max: 0 }]
  assert.equal(colorReduction(frame, colors).rows, survivors(frame, colors).length)
})

test('random edits keep the chosen maximum and the listed rows', () => {
  let seed = 19
  const next = limit => {
    seed = (seed * 1103515245 + 12345) % 2147483648
    return seed % limit
  }
  const frame = full(5)
  const keys = frame.flatMap((outcomes, game) => outcomes.map(outcome => `${game}:${outcome}`))
  for (let trial = 0; trial < 40; trial += 1) {
    const holds = [blankHold(), blankHold(), blankHold(), blankHold()]
    const marks = [[], [], [], []]
    for (let edit = 0; edit < 30; edit += 1) {
      const color = next(4)
      const roll = next(5)
      const ceiling = reachOf(marks[color])
      if (roll < 3 && ceiling) {
        const field = roll === 2 ? 'min' : 'max'
        holds[color] = stepHold(holds[color], ceiling, field, next(2) ? 1 : -1)
      } else {
        const key = keys[next(keys.length)]
        marks[color] = marks[color].includes(key)
          ? marks[color].filter(mark => mark !== key)
          : [...marks[color], key]
      }
      const shown = shownLimit(holds[color], reachOf(marks[color]))
      assert.ok(shown.min <= shown.max)
      assert.ok(shown.max <= reachOf(marks[color]))
      if (holds[color].cap != null) assert.ok(shown.max <= holds[color].cap)
    }
    const colors = marks
      .map((list, index) => ({ signs: signsOf(list), ...shownLimit(holds[index], reachOf(list)) }))
      .filter(color => color.signs.length)
    if (!colors.length) continue
    const want = survivors(frame, colors).length
    try {
      assert.equal(colorReduction(frame, colors).rows, want)
    } catch (error) {
      assert.ok(error instanceof ReductionError)
      assert.equal(error.rows || 0, want)
    }
  }
})

test('a color that keeps every row is left off the coupon', () => {
  const reduced = colorReduction([['1', 'X'], ['2'], ['1']], [{ signs: [{ game: 0, outcome: '1' }], min: 0, max: 1 }])
  assert.deepEqual(reduced, { rows: 2, frameRows: 2, system: ['1X', '2', '1'], colors: [] })
})
