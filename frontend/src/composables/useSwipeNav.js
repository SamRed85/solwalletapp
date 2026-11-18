import { onBeforeUnmount, ref, unref, watch } from 'vue'
import { classifyEnd, classifyMove, classifyStart, resolveSwipe } from '@/utils/swipeNav'
import { bindSwipeTabs, swipeTabs } from '@/utils/swipeTabs'

const CONTROL = 'input, textarea, select, label, [contenteditable="true"], [role="slider"]'

function horizontalScroller (node, edge) {
  let current = node instanceof Element ? node : null
  while (current && current !== document.body) {
    const overflow = getComputedStyle(current).overflowX
    if ((overflow === 'auto' || overflow === 'scroll') && current.scrollWidth > current.clientWidth + 1) {
      if (edge === 'back' && current.scrollLeft + current.clientWidth < current.scrollWidth - 1) return true
      if (edge === 'forward' && current.scrollLeft > 1) return true
    }
    current = current.parentElement
  }
  return false
}

function blocked (target) {
  if (!(target instanceof Element)) return true
  if (target.closest(CONTROL)) return true
  if (target.closest('.v-overlay--active, .phone-nav, .sol-bar')) return true
  return false
}

export function useSwipeNav ({ enabled, canBack, onBack }) {
  const offset = ref(0)
  const dragging = ref(false)
  const settling = ref(false)
  let start = null
  let phase = 'pending'
  let ticket = 0

  function reset () {
    swipeTabs()?.settle()
    start = null
    phase = 'pending'
    dragging.value = false
    settling.value = false
    offset.value = 0
  }

  function onStart (event) {
    if (!enabled() || event.touches.length !== 1) return
    const touch = event.touches[0]
    const edge = classifyStart(touch.clientX, window.innerWidth)
    const tabs = swipeTabs()
    if ((!edge && !tabs) || blocked(event.target)) return
    if (!tabs) {
      if (edge !== 'back' || horizontalScroller(event.target, edge) || !canBack()) return
    }
    ticket += 1
    start = { x: touch.clientX, y: touch.clientY, edge, target: event.target }
    phase = 'pending'
  }

  function onMove (event) {
    if (!start || event.touches.length !== 1) return
    const touch = event.touches[0]
    const dx = touch.clientX - start.x
    const dy = touch.clientY - start.y
    if (phase === 'scroll') return
    if (phase !== 'drag') {
      if (Math.abs(dx) < 10 && Math.abs(dy) < 10) return
      if (Math.abs(dy) >= Math.abs(dx)) {
        phase = 'scroll'
        return
      }
      const tabs = swipeTabs()
      const intent = resolveSwipe({
        dx,
        edge: start.edge,
        canNext: !!tabs?.canNext(),
        canPrev: !!tabs?.canPrev(),
        canBack: canBack()
      })
      const along = intent === 'prev' ? 'forward' : 'back'
      if (!intent || horizontalScroller(start.target, along)) {
        phase = 'scroll'
        return
      }
      start.intent = intent
      phase = 'drag'
    }
    start.travel = dx
    if (start.intent === 'next' || start.intent === 'prev') {
      swipeTabs()?.move(dx)
      if (event.cancelable) event.preventDefault()
      return
    }
    const next = classifyMove('back', dx, dy, 'drag')
    phase = next.phase
    if (phase !== 'drag') return
    dragging.value = true
    offset.value = next.offset
    if (event.cancelable) event.preventDefault()
  }

  function onEnd () {
    if (!start) return
    const intent = start.intent
    const travel = intent === 'back' ? offset.value : (start.travel || 0)
    const commit = phase === 'drag' && classifyEnd(intent, travel, window.innerWidth)
    const tabWay = (commit === 'next' && travel < 0) || (commit === 'prev' && travel > 0)
    if (tabWay) {
      if (commit === 'next') swipeTabs()?.next()
      else swipeTabs()?.prev()
      reset()
      return
    }
    if (commit === 'back') {
      const mine = ticket
      settling.value = true
      dragging.value = false
      offset.value = -window.innerWidth
      window.setTimeout(() => {
        if (mine !== ticket) return
        onBack()
        reset()
      }, window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 180)
      start = null
      return
    }
    reset()
  }

  function listen () {
    window.addEventListener('touchstart', onStart, { passive: true })
    window.addEventListener('touchmove', onMove, { passive: false })
    window.addEventListener('touchend', onEnd)
    window.addEventListener('touchcancel', onEnd)
  }

  function silence () {
    window.removeEventListener('touchstart', onStart)
    window.removeEventListener('touchmove', onMove)
    window.removeEventListener('touchend', onEnd)
    window.removeEventListener('touchcancel', onEnd)
    reset()
  }

  watch(enabled, on => {
    silence()
    if (on) listen()
  }, { immediate: true })

  onBeforeUnmount(silence)

  return { offset, dragging, settling }
}

export function useTabSwipe (current, items) {
  const values = () => {
    const list = typeof items === 'function' ? items() : unref(items)
    return list.map(item => item && typeof item === 'object' ? item.id : item)
  }
  const drag = ref(0)
  const dragging = ref(false)
  const actions = {
    canNext: () => {
      const index = values().indexOf(unref(current))
      return index >= 0 && index < values().length - 1
    },
    canPrev: () => values().indexOf(unref(current)) > 0,
    next () {
      const list = values()
      const index = list.indexOf(unref(current))
      if (index >= 0 && index < list.length - 1) current.value = list[index + 1]
    },
    prev () {
      const list = values()
      const index = list.indexOf(unref(current))
      if (index > 0) current.value = list[index - 1]
    },
    move (dx) {
      drag.value = dx
      dragging.value = true
    },
    settle () {
      dragging.value = false
      drag.value = 0
    }
  }
  const release = bindSwipeTabs(actions)
  onBeforeUnmount(() => {
    actions.settle()
    release()
  })
  return { drag, dragging }
}
