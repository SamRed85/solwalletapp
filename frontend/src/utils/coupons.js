const outcomeOrder = ['1', 'X', '2']

// A system coupon stores one or more outcomes per game. The paid rows are every
// combination. The count is the product, so 44,000 rows are never built in the browser.
export function combinationCount (picks) {
  if (!picks.length || picks.some(game => game.length === 0)) return 0
  return picks.reduce((total, game) => total * game.length, 1)
}

export function systemRows (picks) {
  return picks.map(game => outcomeOrder.filter(outcome => game.includes(outcome)).join(''))
}

export function expandCombinations (picks) {
  const count = combinationCount(picks)
  if (count === 0 || count > 8) return []
  const ordered = picks.map(game => outcomeOrder.filter(outcome => game.includes(outcome)))
  return ordered.reduce(
    (rows, game) => rows.flatMap(row => game.map(outcome => row + outcome)),
    ['']
  )
}
