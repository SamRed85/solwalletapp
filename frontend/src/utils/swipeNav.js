export const SWIPE_EDGE = 28

export function classifyStart (x, width, edge = SWIPE_EDGE) {
  if (!Number.isFinite(x) || !Number.isFinite(width) || width <= edge * 2) return null
  if (x >= width - edge) return 'back'
  return null
}

export function classifyMove (edge, dx, dy, phase) {
  if (phase === 'scroll' || edge == null) return { phase: 'scroll', offset: 0 }
  const ax = Math.abs(dx)
  const ay = Math.abs(dy)
  if (phase !== 'drag') {
    if (ax < 10 && ay < 10) return { phase: 'pending', offset: 0 }
    if (ay >= ax) return { phase: 'scroll', offset: 0 }
    if (edge === 'back' && dx >= 0) return { phase: 'scroll', offset: 0 }
    if (edge === 'forward' && dx <= 0) return { phase: 'scroll', offset: 0 }
  }
  const offset = edge === 'back' ? Math.min(0, dx) : Math.max(0, dx)
  return { phase: 'drag', offset }
}

export function classifyEnd (edge, offset, width) {
  const need = Math.min(88, Math.max(56, width * 0.22))
  return Math.abs(offset) >= need ? edge : 'cancel'
}

// A leftward swipe selects the tab to the right. A rightward swipe selects the
// tab to the left. Leaving a page only happens from the right edge, and only
// on a page that was opened from another.
export function resolveSwipe ({ dx, edge, canNext, canPrev, canBack }) {
  if (dx < 0 && canNext) return 'next'
  if (dx > 0 && canPrev) return 'prev'
  if (edge === 'back' && dx < 0 && canBack) return 'back'
  return null
}
