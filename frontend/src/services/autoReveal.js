import { watch } from 'vue'
import { eventApi } from './api'
import { COUPON_PLACED, autoRevealCoupons } from './entry'
import { eventPda } from './program'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'

// setTimeout cannot wait longer than about 24 days, and the app may sleep, so long waits are split.
const MAX_WAIT_MS = 30 * 60 * 1000
const RETRY_MS = 30 * 1000
const MAX_FAILURES = 5
// The chain clock can trail the browser clock by a few seconds.
const CLOCK_MARGIN_MS = 5000

let timer = null
let running = false
let failures = 0
// Coupons whose reveal the player declined in Phantom are left to the manual button.
let declined = new Set()

function schedule (ms) {
  clearTimeout(timer)
  timer = auth.wallet ? setTimeout(run, Math.max(0, Math.min(ms, MAX_WAIT_MS))) : null
}

async function vaultEvents () {
  const events = new Map()
  for (const event of await eventApi.list()) {
    if (event.chain) events.set(event.chain.address || eventPda(event.chain.admin, event.chain.nonce), event)
  }
  return events
}

async function run () {
  if (running || !auth.wallet) return
  running = true
  try {
    await loadConfig()
    // The keeper publishes the picks when entries close. This signs only if that has not landed.
    const result = await autoRevealCoupons({
      config: auth.config, wallet: auth.wallet, events: await vaultEvents(), skip: declined, lagSeconds: 90
    })
    failures = 0
    if (result.revealed) notify(`Revealed ${result.revealed} coupon(s) now that entries have closed`)
    schedule(result.nextDeadline ? result.nextDeadline * 1000 - Date.now() + CLOCK_MARGIN_MS : MAX_WAIT_MS)
  } catch (error) {
    if (/reject|declined|cancel/i.test(error.message)) {
      for (const address of error.due ?? []) declined.add(address)
      notify('Automatic reveal skipped. Reveal the coupons from My Coupons before the games end.', 'info')
      schedule(MAX_WAIT_MS)
    } else if (++failures <= MAX_FAILURES) {
      schedule(RETRY_MS)
    } else {
      notifyError(new Error(`Automatic reveal failed: ${error.message}`))
      failures = 0
      schedule(MAX_WAIT_MS)
    }
  } finally {
    running = false
  }
}

export function startAutoReveal () {
  watch(() => auth.wallet, () => {
    declined = new Set()
    failures = 0
    schedule(0)
  }, { immediate: true })
  window.addEventListener(COUPON_PLACED, () => schedule(0))
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') schedule(0)
  })
}
