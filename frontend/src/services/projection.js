import { correctCounts, picksFromSystem } from './program.js'

// FT and half time use the status. Added time is shown as 90+'. Otherwise the minute.
export function matchClock (game) {
  if (!game) return ''
  if (game.status === 'HT' || game.status === 'FT' || game.status === 'AET' || game.status === 'PEN') return game.status
  if (game.elapsed != null && game.extra > 0) return `${game.elapsed}+'`
  if (game.elapsed != null) return `${game.elapsed}'`
  return game.status || ''
}

export function kickoffLabel (kickoff, now = new Date()) {
  const date = new Date(kickoff)
  if (Number.isNaN(date.getTime())) return ''
  const time = date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  const sameDay = date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate()
  if (sameDay) return `Today ${time}`
  const day = date.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
  return `${day} ${time}`
}

// Standing results are the live 1, X, 2 for each game. A match that has not started is X.
export function standingResults (projection) {
  const games = projection?.games
  if (!games?.length || games.some(game => !game.projected)) return ''
  return games.map(game => game.projected).join('')
}

// The figure is what this coupon would pay if the current scores do not change.
// Before the reveal window closes it is the maximum for this coupon alone.
export function projectCoupon (system, projection) {
  if (!projection?.complete || !system?.length || system.some(game => !game)) return null
  if (projection.games.length !== system.length) return null
  const results = projection.games.map(game => game.projected).join('')
  const counts = correctCounts(picksFromSystem(system), results)
  let best = 0
  for (let score = counts.length - 1; score > 0; score--) {
    if (counts[score]) {
      best = score
      break
    }
  }
  const lines = (projection.tiers ?? []).flatMap(tier => {
    const rows = counts[tier.correct] ?? 0
    if (!rows) return []
    const pot = BigInt(tier.potLamports || 0)
    const each = projection.maximum ? pot / BigInt(rows) : BigInt(tier.perWinnerLamports || 0)
    return [{ correct: tier.correct, rows, each }]
  }).sort((a, b) => b.correct - a.correct)
  const total = lines.reduce((sum, line) => sum + line.each * BigInt(line.rows), 0n)
  return { best, games: system.length, lines, total, maximum: !!projection.maximum }
}
