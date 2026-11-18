export function blankHold () {
  return { min: 0, cap: null }
}

// `cap` null means the maximum stays on every sign. A chosen cap is remembered when
// signs are removed and put back, so a tight maximum does not start following again.
export function shownLimit (hold, markCount) {
  const max = hold.cap == null ? markCount : Math.min(hold.cap, markCount)
  return { min: Math.min(hold.min, max), max }
}

export function stepHold (hold, markCount, field, delta) {
  const shown = shownLimit(hold, markCount)
  const next = shown[field] + delta
  if (next < 0 || next > markCount) return hold
  if (field === 'max') return { min: Math.min(hold.min, next), cap: next === markCount ? null : next }
  const max = hold.cap == null ? markCount : Math.min(hold.cap, markCount)
  if (next <= max) return { ...hold, min: next }
  return { min: next, cap: next === markCount ? null : next }
}
