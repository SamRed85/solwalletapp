<template>
  <v-container class="board py-4" max-width="480">
    <h1 class="board-title">Leaderboard</h1>
    <p class="board-note">Settled winnings. On-chain rounds appear after their results are saved.</p>

    <div class="spans">
      <button v-for="item in spans" :key="item.id" type="button" class="span" :class="{ on: span === item.id }" @click="span = item.id">
        {{ item.label }}
      </button>
    </div>

    <v-btn v-if="!auth.wallet" class="mt-4" variant="outlined" :loading="auth.busy" @click="onLogin">Connect to see your rank</v-btn>

    <TabReel :items="spans" :current="span" :drag="drag" :dragging="dragging">
      <template #default="{ item }">
        <div v-if="auth.wallet" class="me">
          <span class="avatar">{{ initials(auth.displayName || auth.wallet) }}</span>
          <div class="me-copy">
            <div class="me-label">Your rank</div>
            <div class="me-rank"><span>#</span> {{ boards[item.id]?.me?.rank || '–' }}</div>
          </div>
          <div class="me-won">{{ boards[item.id]?.me?.winningsLamports ? `+${formatSol(boards[item.id].me.winningsLamports)}` : '—' }}</div>
        </div>
        <v-progress-linear v-if="loading && !boards[item.id]" indeterminate class="mt-4" />
        <p v-else-if="failed[item.id] && !boards[item.id]" class="empty">The leaderboard could not be loaded. Restart the backend so this server has the new route.</p>
        <p v-else-if="boards[item.id] && !boards[item.id].rows.length" class="empty">No settled winnings in this period.</p>
        <div v-for="(row, index) in boards[item.id]?.rows || []" :key="row.wallet" class="player">
          <span class="place" :class="placeClass(index)">{{ index + 1 }}.</span>
          <span class="avatar">{{ initials(row.name || row.wallet) }}</span>
          <div class="player-copy">
            <div class="player-name">{{ row.name || shortAddress(row.wallet) }}</div>
            <span class="hit">{{ row.bestCorrect }} of {{ row.gameCount }}</span>
          </div>
          <div class="player-won">
            <div>+{{ formatSol(row.winningsLamports) }}</div>
            <div class="coupons">{{ row.coupons }} coupon{{ row.coupons === 1 ? '' : 's' }}</div>
          </div>
        </div>
      </template>
    </TabReel>
  </v-container>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import TabReel from '@/components/TabReel.vue'
import { useTabSwipe } from '@/composables/useSwipeNav'
import { eventApi } from '@/services/api'
import { auth, openLogin } from '@/stores/auth'
import { formatSol, shortAddress } from '@/utils/format'

const spans = [
  { id: 'round', label: 'Round' },
  { id: 'week', label: 'Week' },
  { id: 'month', label: 'Month' },
  { id: 'all', label: 'All' }
]
const span = ref('week')
const { drag, dragging } = useTabSwipe(span, spans)
const boards = ref({})
const loading = ref(false)
const failed = ref({})

function initials (value) {
  return ((value || '').trim().slice(0, 2) || '•').toUpperCase()
}

function placeClass (index) {
  return ['gold', 'silver', 'bronze'][index] || ''
}

async function load (id = span.value) {
  const visible = id === span.value && !boards.value[id]
  if (visible) loading.value = true
  try {
    const next = await eventApi.leaderboard(id)
    boards.value = { ...boards.value, [id]: { rows: next.rows || [], me: next.me || null } }
    const nextFailed = { ...failed.value }
    delete nextFailed[id]
    failed.value = nextFailed
  } catch {
    if (!boards.value[id]) failed.value = { ...failed.value, [id]: true }
  } finally {
    if (visible) loading.value = false
  }
}

function refresh () {
  boards.value = {}
  failed.value = {}
  for (const item of spans) load(item.id)
}

function onLogin () {
  openLogin()
}

onMounted(refresh)
watch(span, id => {
  if (!boards.value[id]) load(id)
})
watch(() => auth.wallet, refresh)
</script>

<style scoped>
.board-title { margin: 8px 0 0; text-align: center; font-size: 30px; line-height: 36px; font-weight: 700; letter-spacing: -0.02em; }
.board-note { margin: 8px 0 0; text-align: center; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.spans { display: flex; gap: 4px; margin-top: 18px; }
.span { height: 26px; padding: 0 8px; border: 0; border-radius: 8px; background: transparent; color: rgba(var(--v-theme-on-surface), 0.62); font: inherit; font-size: var(--fs-small); font-weight: 600; cursor: pointer; }
.span.on { background: rgb(var(--v-theme-surface-variant)); color: rgb(var(--v-theme-on-surface)); }
.me { display: flex; align-items: center; gap: 12px; margin-top: 14px; padding: 14px; border-radius: 18px; background: rgb(var(--v-theme-surface)); }
.me-copy { flex: 1; }
.me-label { font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.me-rank { font-size: var(--fs-title); line-height: var(--lh-title); font-weight: 700; }
.me-rank span { color: rgb(var(--v-theme-primary)); }
.me-won { font-size: var(--fs-heading); font-weight: 700; color: rgb(var(--v-theme-success)); }
.avatar { width: 42px; height: 42px; border-radius: 999px; background: rgb(var(--v-theme-primary)); color: #fff; display: inline-flex; align-items: center; justify-content: center; font-weight: 800; flex: none; }
.empty { margin-top: 28px; color: rgba(var(--v-theme-on-surface), 0.62); }
.player { display: flex; align-items: center; gap: 12px; height: 66px; }
.place { width: 24px; font-weight: 700; color: rgba(var(--v-theme-on-surface), 0.62); }
.place.gold { color: #E9B949; }
.place.silver { color: #A7A9B8; }
.place.bronze { color: #C9814F; }
.player-copy { flex: 1; min-width: 0; }
.player-name { font-weight: 600; }
.hit { display: inline-block; margin-top: 3px; padding: 1px 8px; border-radius: 8px; background: rgb(var(--v-theme-surface-variant)); font-size: 11px; font-weight: 700; }
.player-won { text-align: right; font-weight: 700; color: rgb(var(--v-theme-success)); }
.coupons { font-size: var(--fs-small); font-weight: 500; color: rgba(var(--v-theme-on-surface), 0.45); }
</style>
