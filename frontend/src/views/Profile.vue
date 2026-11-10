<template>
  <v-container class="profile py-4" max-width="480">
    <div class="identity">
      <span class="avatar">{{ initials }}</span>
      <v-btn color="primary" class="claim" :to="'/coupons'">{{ claimLabel }}</v-btn>
    </div>
    <h1 class="name">{{ auth.displayName || 'Unnamed' }}</h1>
    <code class="wallet">{{ short }}</code>

    <div class="rename">
      <v-text-field v-model="name" label="Profile name" :rules="[nameRule]" counter="32" maxlength="32" density="comfortable" hide-details="auto" />
      <v-btn color="primary" variant="text" :disabled="!changed" :loading="saving" @click="save">Save</v-btn>
    </div>

    <div class="stats">
      <span><b>{{ coupons }}</b> Coupons</span>
      <span><b>{{ wins }}</b> Wins</span>
      <span><b>{{ rankLabel }}</b> This week</span>
    </div>
    <p class="facts">Best {{ bestLabel }}</p>

    <div class="net-head">
      <div>
        <div class="net">{{ netLabel }}</div>
        <div class="net-note">net result</div>
      </div>
      <div class="spans">
        <button v-for="item in spans" :key="item" type="button" class="span" :class="{ on: span === item }" @click="span = item">{{ item }}</button>
      </div>
    </div>
    <svg class="chart" viewBox="0 0 288 110" role="img" :aria-label="`Net result, ${span}`">
      <line x1="0" :x2="288" :y1="zeroY" :y2="zeroY" class="zero" />
      <polyline :points="points" fill="none" stroke="rgb(var(--v-theme-primary))" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" />
    </svg>

    <div class="currency">
      <div class="net-note">Display currency</div>
      <p class="facts">This only changes prices on screen. Coupons are still paid and settled in SOL.</p>
      <v-btn-toggle v-model="currency" class="currency-toggle" mandatory divided color="primary">
        <v-btn value="SOL">SOL</v-btn>
        <v-btn value="USD">USD</v-btn>
        <v-btn value="SEK">SEK</v-btn>
      </v-btn-toggle>
      <p v-if="rateLine" class="facts">{{ rateLine }}</p>
    </div>

    <v-btn color="primary" size="large" block class="play" :disabled="!openEvent" :to="openEvent ? `/events/${openEvent.id}` : undefined">
      {{ openEvent ? 'Play next round' : 'No open round' }}
    </v-btn>
  </v-container>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { authApi, entryApi, eventApi } from '@/services/api'
import { auth, setProfile } from '@/stores/auth'
import { display, setCurrency } from '@/stores/currency'
import { notify, notifyError } from '@/stores/notify'
import { formatSol, shortAddress } from '@/utils/format'

const name = ref(auth.displayName)
const saving = ref(false)
const entries = ref([])
const rank = ref(0)
const openEvent = ref(null)
const span = ref('All')
const spans = ['Round', 'Week', 'All']

const currency = computed({
  get: () => display.currency,
  set: setCurrency
})
const rateLine = computed(() => {
  if (!display.rates) return ''
  const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(display.rates.usd)
  const sek = new Intl.NumberFormat('sv-SE', { style: 'currency', currency: 'SEK', maximumFractionDigits: 2 }).format(display.rates.sek)
  return `About ${usd} · ${sek} per SOL`
})

watch(() => auth.displayName, value => { name.value = value })

const normalized = value => (value || '').trim().replace(/\s+/g, ' ')
const nameRule = value => {
  const next = normalized(value)
  return next.length >= 2 && next.length <= 32 ? true : 'Use 2 to 32 characters'
}
const changed = computed(() => nameRule(name.value) === true && normalized(name.value) !== auth.displayName)
const initials = computed(() => ((auth.displayName || auth.wallet || '').trim().slice(0, 2) || '•').toUpperCase())
const short = computed(() => shortAddress(auth.wallet))

const settled = computed(() => entries.value.filter(entry => entry.event?.settlement))
const inSpan = computed(() => {
  if (span.value === 'All') return settled.value
  if (span.value === 'Round') {
    const newest = [...settled.value].sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt))[0]
    return newest ? settled.value.filter(entry => entry.eventId === newest.eventId) : []
  }
  const start = Date.now() - 7 * 24 * 60 * 60 * 1000
  return settled.value.filter(entry => new Date(entry.createdAt).getTime() >= start)
})
const coupons = computed(() => entries.value.length)
const wins = computed(() => entries.value.filter(entry => entry.winningsLamports > 0).length)
const claimLabel = computed(() => wins.value ? 'See winnings' : 'My coupons')
const best = computed(() => {
  let correct = 0
  let games = 0
  for (const entry of entries.value) {
    const hit = Math.max(0, ...(entry.scores || []).map(band => band.correct), ...(entry.results || []).map(row => row.correct))
    if (hit > correct) {
      correct = hit
      games = entry.event?.gameCount || 0
    }
  }
  return { correct, games }
})
const bestLabel = computed(() => best.value.games ? `${best.value.correct} of ${best.value.games}` : '—')
const rankLabel = computed(() => rank.value ? `#${rank.value}` : '—')

const series = computed(() => {
  const ordered = [...inSpan.value].sort((a, b) => new Date(a.createdAt) - new Date(b.createdAt))
  let net = 0
  const values = [0]
  for (const entry of ordered) {
    net += (entry.winningsLamports || 0) - (entry.lamports || 0)
    values.push(net)
  }
  return values
})
const netLabel = computed(() => {
  const net = series.value[series.value.length - 1] || 0
  const sign = net > 0 ? '+' : net < 0 ? '−' : ''
  return `${sign}${formatSol(Math.abs(net))}`
})
const points = computed(() => {
  const values = series.value
  const min = Math.min(...values, 0)
  const max = Math.max(...values, 0)
  const spanHeight = max - min || 1
  return values.map((value, index) => {
    const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 288
    const y = 102 - ((value - min) / spanHeight) * 86
    return `${x},${y}`
  }).join(' ')
})
const zeroY = computed(() => {
  const values = series.value
  const min = Math.min(...values, 0)
  const max = Math.max(...values, 0)
  return 102 - ((0 - min) / (max - min || 1)) * 86
})

async function save () {
  saving.value = true
  try {
    setProfile(await authApi.updateProfile(name.value))
    notify('Profile name saved')
  } catch (error) {
    notifyError(error)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    entries.value = await entryApi.mine()
  } catch (error) {
    notifyError(error)
  }
  try {
    const board = await eventApi.leaderboard('week')
    rank.value = board.me?.rank || 0
  } catch {
    rank.value = 0
  }
  try {
    openEvent.value = (await eventApi.list()).find(event => event.status === 'open') || null
  } catch {
    openEvent.value = null
  }
})
</script>

<style scoped>
.identity { display: flex; align-items: flex-end; justify-content: space-between; }
.avatar { width: 72px; height: 72px; border-radius: 999px; background: rgb(var(--v-theme-primary)); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 26px; font-weight: 800; }
.claim { border-radius: 14px; }
.name { margin: 12px 0 0; font-size: var(--fs-title); line-height: var(--lh-title); font-weight: 700; }
.wallet { display: block; margin-top: 2px; color: rgba(var(--v-theme-on-surface), 0.62); font-size: var(--fs-small); }
.rename { display: flex; align-items: center; gap: 8px; margin-top: 16px; }
.rename .v-input { flex: 1; }
.stats { display: flex; gap: 16px; margin-top: 16px; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.stats b { color: rgb(var(--v-theme-on-surface)); }
.facts { margin: 8px 0 0; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.45); }
.net-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-top: 24px; }
.net { font-size: var(--fs-display); line-height: var(--lh-display); font-weight: 700; letter-spacing: -0.02em; }
.net-note { font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.spans { display: flex; gap: 2px; }
.span { height: 26px; padding: 0 8px; border: 0; border-radius: 8px; background: transparent; color: rgba(var(--v-theme-on-surface), 0.62); font: inherit; font-size: var(--fs-small); font-weight: 600; cursor: pointer; }
.span.on { background: rgb(var(--v-theme-surface-variant)); color: rgb(var(--v-theme-on-surface)); }
.chart { display: block; width: 100%; height: 110px; margin-top: 12px; }
.zero { stroke: rgba(var(--v-theme-on-surface), 0.28); stroke-dasharray: 2 4; }
.currency { margin-top: 20px; }
.currency-toggle { display: flex; width: 100%; margin-top: 8px; }
.currency-toggle :deep(.v-btn) { flex: 1; }
.play { margin-top: 28px; border-radius: 14px; }
</style>
