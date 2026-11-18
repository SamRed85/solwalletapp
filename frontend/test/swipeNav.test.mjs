import assert from 'node:assert/strict'
import test from 'node:test'
import { classifyEnd, classifyMove, classifyStart, resolveSwipe } from '../src/utils/swipeNav.js'

test('a finger on the right edge starts a back swipe', () => {
  assert.equal(classifyStart(370, 390), 'back')
  assert.equal(classifyStart(8, 390), null)
  assert.equal(classifyStart(120, 390), null)
})

test('a vertical move keeps scrolling, and a sideways move drags the page', () => {
  assert.equal(classifyMove('back', -4, 3, 'pending').phase, 'pending')
  assert.equal(classifyMove('back', -6, 30, 'pending').phase, 'scroll')
  const drag = classifyMove('back', -40, 8, 'pending')
  assert.equal(drag.phase, 'drag')
  assert.equal(drag.offset, -40)
  assert.equal(classifyMove('back', 40, 4, 'pending').phase, 'scroll')
  assert.equal(classifyMove('forward', 50, 6, 'pending').offset, 50)
})

test('a left swipe selects the tab to the right, and only an opened page can go back', () => {
  const open = { canNext: true, canPrev: false, canBack: false }
  assert.equal(resolveSwipe({ dx: -40, edge: null, ...open }), 'next')
  assert.equal(resolveSwipe({ dx: 40, edge: null, ...open }), null)
  const live = { canNext: true, canPrev: true, canBack: false }
  assert.equal(resolveSwipe({ dx: -40, edge: 'back', ...live }), 'next')
  assert.equal(resolveSwipe({ dx: 40, edge: null, ...live }), 'prev')
  const detail = { canNext: false, canPrev: false, canBack: true }
  assert.equal(resolveSwipe({ dx: -40, edge: 'back', ...detail }), 'back')
  assert.equal(resolveSwipe({ dx: 40, edge: null, ...detail }), null)
  assert.equal(resolveSwipe({ dx: -40, edge: null, ...detail }), null)
})

test('the page changes only after the finger has travelled far enough', () => {
  assert.equal(classifyEnd('back', -40, 390), 'cancel')
  assert.equal(classifyEnd('back', -100, 390), 'back')
  assert.equal(classifyEnd('forward', 100, 390), 'forward')
})
