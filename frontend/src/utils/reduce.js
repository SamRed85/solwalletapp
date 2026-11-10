import { combinationCount } from './coupons.js'

export const MAX_COMBINATIONS = 44000
export const MAX_COLOR_STATES = 64
export const COLOR_IDS = ['purple', 'yellow', 'red', 'green']
export const COLOR_LABELS = { purple: 'Purple', yellow: 'Yellow', red: 'Red', green: 'Green' }
export const COLOR_INK = { purple: '#6A1B9A', yellow: '#F9A825', red: '#C62828', green: '#2E7D32' }

// A pick with one color is a solid fill. Several colors share the button in equal bands,
// in the same order on the coupon and on the receipt. The bands are measured from the
// border edge, so a button with a border does not repeat the fill under it.
export function colorPaint (ids) {
  const ink = ids.map(id => COLOR_INK[id]).filter(Boolean)
  if (!ink.length) return undefined
  const text = ink.length === 1 && ids[0] === 'yellow' ? '#1a1a1a' : '#fff'
  if (ink.length === 1) return { background: ink[0], borderColor: ink[0], color: text }
  const step = 100 / ink.length
  const stops = ink.map((color, index) => `${color} ${index * step}% ${(index + 1) * step}%`).join(', ')
  return { background: `linear-gradient(135deg, ${stops}) border-box`, borderColor: 'transparent', color: '#fff' }
}

// The colors saved on a coupon, in display order. `signs` is the program's bitset:
// game g outcome k (1, X, 2) is bit 3g+k. The program does not store which color the
// player chose, so a coupon saved without `id` falls back to the order of the colors.
export function colorView (colors = []) {
  return colors.map((color, index) => {
    const signs = BigInt(color.signs)
    const marks = []
    for (let bit = 0n; signs >> bit; bit += 1n) {
      if ((signs & (1n << bit)) === 0n) continue
      marks.push({ game: Number(bit / 3n), outcome: OUTCOMES[Number(bit % 3n)] })
    }
    const id = COLOR_IDS.includes(color.id) ? color.id : (COLOR_IDS[index] || COLOR_IDS[COLOR_IDS.length - 1])
    return { id, label: COLOR_LABELS[id] || 'Color', min: color.min, max: color.max, marks }
  }).filter(color => color.marks.length)
}

const OUTCOMES = ['1', 'X', '2']

// Games whose color landed outside its min and max on this result.
// A color inside its range contributes no games, so a hit color stays green.
export function missedColorGames (colors, results) {
  const missed = new Set()
  if (!results) return missed
  for (const color of colorView(colors)) {
    const hits = color.marks.filter(mark => {
      const result = results[mark.game]
      return result === 'C' || result === mark.outcome
    }).length
    if (hits < color.min || hits > color.max) {
      for (const mark of color.marks) missed.add(mark.game)
    }
  }
  return missed
}

// `rows` and `frameRows` ride along when the colors were counted but the result is refused, so
// the page can show how far the reduction got instead of nothing.
export class ReductionError extends Error {
  constructor (message, { rows = 0, frameRows = 0 } = {}) {
    super(message)
    this.rows = rows
    this.frameRows = frameRows
  }
}

function frameOf (picks) {
  return picks.map(game => OUTCOMES.filter(outcome => [...game].includes(outcome)))
}

function signKey (game, outcome) {
  return `${game}:${outcome}`
}

function activeColors (frame, colors) {
  if (!colors?.length) return []
  if (colors.length > COLOR_IDS.length) throw new ReductionError('At most 4 colors')
  return colors.map(color => {
    if (!Number.isInteger(color.min) || !Number.isInteger(color.max) || color.min < 0 || color.max < color.min || color.max > frame.length) {
      throw new ReductionError('Each color needs a min that is not above its max')
    }
    const signs = new Set()
    for (const sign of color.signs || []) {
      if (!Number.isInteger(sign.game) || sign.game < 0 || sign.game >= frame.length || !OUTCOMES.includes(sign.outcome)) {
        throw new ReductionError('A color sign has to be 1, X or 2 on one of the games')
      }
      if (frame[sign.game].includes(sign.outcome)) signs.add(signKey(sign.game, sign.outcome))
    }
    return { id: color.id, min: color.min, max: color.max, signs }
  }).filter(color => color.signs.size > 0 || color.min > 0)
}

// Game g outcome k is bit 3g+k, in the order 1, X, 2, as the program reads it.
function signBits (signs) {
  let bits = 0n
  for (const key of signs) {
    const [game, outcome] = key.split(':')
    bits |= 1n << BigInt(3 * Number(game) + OUTCOMES.indexOf(outcome))
  }
  return bits
}

const RESULT_BITS = { 1: 0b001, X: 0b010, 2: 0b100, C: 0b111 }

// Rows of a pick-mask coupon by number of correct picks, counting only rows inside every
// color's range, as the program does. `colors` holds { signs, min, max } with signs as a
// BigInt or decimal string. With no results every kept row is at zero correct.
export function keptCounts (picks, colors = [], results = '') {
  const games = results ? results.length : picks.length
  const ranges = colors.map(color => ({ signs: BigInt(color.signs), min: color.min, max: color.max }))
  let rows = new Map([[`${ranges.map(() => 0).join(',')}|0`, 1]])
  for (let game = 0; game < games; game += 1) {
    const resultBits = results ? RESULT_BITS[results[game]] ?? 0 : 0
    const next = new Map()
    for (const [key, count] of rows) {
      const [hitText, scoreText] = key.split('|')
      const hits = hitText ? hitText.split(',').map(Number) : []
      for (let outcome = 0; outcome < 3; outcome += 1) {
        if (!(picks[game] & (1 << outcome))) continue
        const bit = 1n << BigInt(3 * game + outcome)
        const moved = hits.slice()
        let allowed = true
        ranges.forEach((range, index) => {
          if (!(range.signs & bit)) return
          moved[index] += 1
          if (moved[index] > range.max) allowed = false
        })
        if (!allowed) continue
        const score = Number(scoreText) + (resultBits & (1 << outcome) ? 1 : 0)
        const movedKey = `${moved.join(',')}|${score}`
        next.set(movedKey, (next.get(movedKey) || 0) + count)
      }
    }
    rows = next
  }
  const counts = new Array(games + 1).fill(0)
  for (const [key, count] of rows) {
    const [hitText, scoreText] = key.split('|')
    const hits = hitText ? hitText.split(',').map(Number) : []
    if (ranges.every((range, index) => hits[index] >= range.min)) counts[Number(scoreText)] += count
  }
  return counts
}

function maskOf (outcomes) {
  return outcomes.reduce((mask, outcome) => mask | (1 << OUTCOMES.indexOf(outcome)), 0)
}

// One coupon for the whole colored frame. The program counts the same kept rows at reveal,
// score and claim, so the price is the kept rows and only one signature is needed.
// A color that keeps every row is left out, and the color states stay within the program's cap.
export function colorReduction (picks, colors, { maxCombinations = MAX_COMBINATIONS } = {}) {
  const frame = frameOf(picks)
  if (!frame.length || frame.some(game => game.length === 0)) throw new ReductionError('Every game needs a pick')
  const used = activeColors(frame, colors)
  const counted = []
  let states = 1
  for (const color of used) {
    const reach = frame.filter((outcomes, game) => outcomes.some(outcome => color.signs.has(signKey(game, outcome)))).length
    if (color.min > reach) throw new ReductionError('This reduction leaves no rows')
    const cap = Math.min(color.max, reach)
    if (color.min === 0 && cap === reach) continue
    states *= cap + 1
    if (states > MAX_COLOR_STATES) {
      throw new ReductionError('These colors are too detailed for one coupon. Narrow a min and max, or use fewer colors.')
    }
    counted.push({ signs: signBits(color.signs), min: color.min, max: color.max, ...(COLOR_IDS.includes(color.id) ? { id: color.id } : {}) })
  }
  const rows = keptCounts(frame.map(maskOf), counted)[0]
  if (!rows) throw new ReductionError('This reduction leaves no rows')
  if (rows > maxCombinations) {
    throw new ReductionError(
      `${rows.toLocaleString()} rows are still kept. A coupon can hold at most ${maxCombinations.toLocaleString()}. Raise a min, lower a max, or remove a pick.`,
      { rows, frameRows: combinationCount(frame) }
    )
  }
  return {
    rows,
    frameRows: combinationCount(frame),
    system: frame.map(game => game.join('')),
    colors: counted
  }
}
