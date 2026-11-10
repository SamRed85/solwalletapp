<template>
  <v-container class="home py-6" max-width="480">
    <div class="hero">
      <div class="hero-copy">
        <div class="balance" :class="balanceClass">
          <template v-if="auth.wallet && auth.balance !== null">
            <SolAmount :lamports="Math.round(auth.balance * 1e9)" />
          </template>
          <template v-else-if="auth.wallet">—</template>
          <template v-else >Connect to play</template>
        </div>
        <p v-if="auth.wallet" class="text-medium-emphasis balance-note">Devnet wallet balance</p>
      </div>
      <v-btn v-if="auth.wallet" color="primary" size="large" class="play" :loading="auth.busy" :disabled="auth.wallet && !openEvent" @click="onPlay">
        {{ openEvent ? 'Play' : 'No open round'}}
      </v-btn>
    </div>

    <section v-if="wins.length" class="mt-6">
      <h2 class="section-title d-flex align-center ga-2">
        <v-icon icon="mdi-lightning-bolt" size="20" />
        Biggest wins this week
      </h2>
      <div class="win-row">
        <article v-for="win in wins" :key="win.wallet" class="win-card">
          <div class="d-flex align-center ga-2">
            <span class="avatar sm">{{ initials(win.name || win.wallet) }}</span>
            <span class="win-name">{{ win.name || shortAddress(win.wallet) }}</span>
          </div>
          <div class="win-amount">+{{ formatSol(win.winningsLamports) }}</div>
          <div class="win-sub">{{ win.eventTitle }} · {{ win.bestCorrect }} of {{ win.gameCount }}</div>
        </article>
      </div>
    </section>

    <div class="tabs" role="tablist">
      <button v-for="item in tabs" :key="item" type="button" role="tab" class="tab" :class="{ on: tab === item }" :aria-selected="tab === item" @click="tab = item">
        {{ item }}
      </button>
    </div>

    <div class="chips">
      <v-btn v-if="auth.isAdmin" icon="mdi-shield-crown" variant="text" size="small" to="/admin" aria-label="Manage events" />
      <button v-for="item in chips" :key="item" type="button" class="chip" :class="{ on: chip === item }" @click="chip = item">
        {{ item === 'jackpot' ? 'Jackpot' : 'All' }}
      </button>
    </div>

    <v-progress-linear v-if="loading" indeterminate class="mt-4" />
    <p v-else-if="!shown.length" class="empty">Nothing in this list.</p>
    <router-link v-for="event in shown" :key="event.id" class="event-row" :to="`/events/${event.id}`">
      <span class="type-mark" :class="{ jackpot: jackpotOf(event) }">
        <v-icon :icon="eventTypeOf(event).icon" :color="jackpotOf(event) ? '#F5B800' : eventTypeOf(event).color" size="22" />
        <span class="type-count">{{ event.gameCount }}</span>
      </span>
      <span class="event-copy">
        <span class="event-title">{{ event.title }}</span>
        <span class="event-meta">{{ meta(event) }}</span>
      </span>
      <span class="event-pot">
        <span>{{ formatSol(potOf(event)) }}</span>
        <span v-if="jackpotOf(event)" class="event-tag is-jackpot">Jackpot {{ formatSol(jackpotOf(event)) }}</span>
        <span v-else class="event-tag" :class="tagClass(event)">{{ tagOf(event) }}</span>
      </span>
    </router-link>

    <section v-if="!auth.wallet" class="connect-note">
      <h2 class="section-title">{{ phantomInstalled ? 'Play when you are ready' : 'Phantom wallet required to play' }}</h2>
      <p class="text-medium-emphasis">
        <template v-if="phantomInstalled">
          You can look through the events without a wallet. Connect Phantom and sign a message when you want to enter a coupon. Signing is free and sends no transaction.
        </template>
        <template v-else>
          Install the Phantom browser extension and switch it to Devnet in its settings. You can still browse the events first.
        </template>
      </p>
      <v-btn v-if="phantomInstalled" color="primary" size="large" :loading="auth.busy" @click="onPlay">Connect wallet</v-btn>
      <v-btn v-else color="primary" size="large" href="https://phantom.app/" target="_blank" prepend-icon="mdi-download">Install Phantom</v-btn>
    </section>
  </v-container>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { eventApi } from '@/services/api'
import { eventPda, fetchEvent } from '@/services/program'
import { isPhantomInstalled } from '@/services/solana'
import { auth, loadConfig, login } from '@/stores/auth'
import { display } from '@/stores/currency'
import { notifyError } from '@/stores/notify'
import { eventTypeOf } from '@/services/eventTypes'
import { formatDate, formatSol, shortAddress } from '@/utils/format'
import SolAmount from '@/components/SolAmount.vue'

const router = useRouter()
const phantomInstalled = ref(isPhantomInstalled())
const events = ref([])
const loading = ref(false)
const chainPots = ref({})
const wins = ref([])
const tab = ref('Open')
const chip = ref('all')
const tabs = ['Open', 'Live', 'Results']

function jackpotOf (event) {
  if (event.chain) return chainPots.value[event.id]?.jackpot || 0
  return event.rolloverInLamports || 0
}

function potOf (event) {
  const chain = event.chain ? chainPots.value[event.id] : null
  if (chain) return chain.sales + chain.jackpot
  return (event.couponsSold || 0) * (event.couponPriceLamports || 0) + (event.rolloverInLamports || 0)
}

const balanceClass = computed(() => {
  if (!auth.wallet || auth.balance == null) return ''
  const code = display.currency
  const rate = code === 'USD' ? display.rates?.usd : code === 'SEK' ? display.rates?.sek : null
  const amount = Math.abs(rate ? auth.balance * rate : Number(auth.balance))
  if (!Number.isFinite(amount)) return ''
  const figures = Math.floor(amount).toString().length
  if (figures > 7) return 'is-compact'
  if (figures > 5) return 'is-tight'
  return ''
})
const openEvent = computed(() => events.value.find(event => event.status === 'open'))
const grouped = computed(() => ({
  Open: events.value.filter(event => event.status === 'open'),
  Live: events.value.filter(event => event.status === 'closed'),
  Results: events.value.filter(event => event.status === 'completed')
}))
const chips = ['all', 'jackpot']
const shown = computed(() => grouped.value[tab.value].filter(event => {
  if (chip.value === 'jackpot') return jackpotOf(event) > 0
  return true
}))
const rollover = computed(() => {
  const event = events.value
    .filter(item => item.status === 'open' && jackpotOf(item) > 0)
    .sort((a, b) => jackpotOf(b) - jackpotOf(a))[0]
  return event ? { title: event.title, amount: jackpotOf(event) } : null
})

watch(tab, () => { chip.value = 'all' })

function meta (event) {
  if (event.status === 'completed') return 'Settled'
  if (event.status === 'closed') return 'In play'
  return `Closes ${formatDate(event.startTime)}`
}

function tagOf (event) {
  if (jackpotOf(event)) return 'Jackpot'
  if (event.status === 'closed') return 'Live'
  if (event.status === 'completed') return 'Result'
  return 'Open'
}

function tagClass (event) {
  if (jackpotOf(event)) return 'is-jackpot'
  if (event.status === 'closed') return 'is-live'
  return ''
}

function initials (value) {
  const name = (value || '').trim()
  return (name.slice(0, 2) || '•').toUpperCase()
}

async function loadChainJackpots (list) {
  const vaults = list.filter(event => event.chain && event.status !== 'draft')
  if (!vaults.length) return
  try {
    const config = await loadConfig()
    const found = {}
    await Promise.all(vaults.map(async event => {
      try {
        const state = await fetchEvent(config.solanaRpcUrl, event.chain.address || eventPda(event.chain.admin, event.chain.nonce))
        found[event.id] = { sales: Number(state.sales), jackpot: Number(state.rolloverIn) }
      } catch {
        // An unreadable vault shows the figures saved with the event.
      }
    }))
    chainPots.value = found
  } catch {
    chainPots.value = {}
  }
}

async function loadWins () {
  try {
    const board = await eventApi.leaderboard('week')
    wins.value = (board.rows || []).slice(0, 2)
  } catch {
    wins.value = []
  }
}

const newestFirst = (a, b) => new Date(b.createdAt || b.startTime) - new Date(a.createdAt || a.startTime)
let phantomTimer

onMounted(() => {
  loadEvents()
  loadWins()
  phantomTimer = setInterval(() => {
    phantomInstalled.value = isPhantomInstalled()
    if (phantomInstalled.value) clearInterval(phantomTimer)
  }, 250)
})
onUnmounted(() => clearInterval(phantomTimer))

async function loadEvents () {
  loading.value = true
  try {
    events.value = (await eventApi.list()).filter(event => event.status !== 'draft' || auth.isAdmin).sort(newestFirst)
    loadChainJackpots(events.value)
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

async function onPlay () {
  if (!auth.wallet) {
    try {
      await login()
    } catch (error) {
      notifyError(error)
    }
    return
  }
  if (openEvent.value) router.push(`/events/${openEvent.value.id}`)
}

watch(() => auth.wallet, () => {
  loadEvents()
  loadWins()
})
</script>

<style scoped>
.home { padding-bottom: 24px; }
.hero { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.hero-copy { min-width: 0; flex: 1 1 auto; }
.balance { font-size: var(--fs-display); line-height: var(--lh-display); font-weight: 700; letter-spacing: -0.02em; white-space: nowrap; }
.balance.is-tight { font-size: var(--fs-title); line-height: var(--lh-title); }
.balance.is-compact { font-size: var(--fs-heading); line-height: var(--lh-heading); }
.balance-note { margin: 2px 0 0; font-size: var(--fs-small); }
.play { min-width: 128px; flex: none; border-radius: 14px; }
.win-row { display: flex; gap: 10px; margin-top: 12px; }
.win-card { flex: 1; min-width: 0; border-radius: 14px; background: rgb(var(--v-theme-surface)); padding: 12px; }
.win-name { font-size: var(--fs-small); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.win-amount { margin-top: 10px; font-size: var(--fs-heading); font-weight: 700; color: rgb(var(--v-theme-success)); }
.win-sub { margin-top: 2px; font-size: 11px; color: rgba(var(--v-theme-on-surface), 0.45); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.avatar { display: inline-flex; align-items: center; justify-content: center; border-radius: 999px; background: rgb(var(--v-theme-primary)); color: #fff; font-weight: 800; flex: none; }
.avatar.sm { width: 24px; height: 24px; font-size: 10px; }
.tabs { display: flex; margin: 16px -16px 0; }
.tab { flex: 1; height: 44px; background: none; border: 0; border-bottom: 2px solid transparent; color: rgba(var(--v-theme-on-surface), 0.45); font: inherit; font-size: var(--fs-body); font-weight: 600; cursor: pointer; }
.tab.on { color: rgb(var(--v-theme-on-surface)); border-bottom-color: rgb(var(--v-theme-primary)); }
.chips { display: flex; gap: 8px; margin-top: 12px; overflow-x: auto; }
.chip { height: 34px; padding: 0 12px; border: 0; border-radius: 10px; background: rgb(var(--v-theme-surface)); color: rgba(var(--v-theme-on-surface), 0.62); font: inherit; font-size: var(--fs-body); font-weight: 600; cursor: pointer; white-space: nowrap; display: inline-flex; align-items: center; gap: 6px; }
.chip.on { background: rgb(var(--v-theme-surface-variant)); color: rgb(var(--v-theme-on-surface)); }
.notice { display: flex; align-items: flex-start; gap: 10px; margin-top: 12px; padding: 12px; border-radius: 14px; background: rgb(var(--v-theme-surface)); }
.notice p { margin: 0; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.notice-strong { color: rgb(var(--v-theme-primary)); font-weight: 600; }
.empty { margin-top: 24px; color: rgba(var(--v-theme-on-surface), 0.62); }
.event-row { display: flex; align-items: center; gap: 12px; height: 64px; color: inherit; text-decoration: none; }
.type-mark { width: 44px; height: 44px; border-radius: 14px; background: rgb(var(--v-theme-surface-variant)); display: flex; align-items: center; justify-content: center; position: relative; flex: none; }
.type-mark.jackpot { background: rgba(245, 184, 0, 0.16); }
.type-count { position: absolute; right: -4px; bottom: -4px; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 999px; background: rgb(var(--v-theme-background)); color: rgb(var(--v-theme-on-surface)); font-size: 10px; font-weight: 800; display: flex; align-items: center; justify-content: center; }
.event-copy { min-width: 0; flex: 1; display: flex; flex-direction: column; }
.event-title, .event-meta { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.event-title { font-size: var(--fs-body); font-weight: 600; }
.event-meta { font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.event-pot { text-align: right; flex: none; display: flex; flex-direction: column; }
.event-pot span:first-child { font-size: var(--fs-body); font-weight: 700; }
.event-tag { font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.event-tag.is-jackpot { color: #F5B800; }
.event-tag.is-live { color: rgb(var(--v-theme-success)); }
.connect-note { margin-top: 28px; display: flex; flex-direction: column; gap: 12px; }
</style>
