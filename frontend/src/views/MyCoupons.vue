<template>
  <v-container class="py-6" max-width="1000">
    <h1 class="text-h4 font-weight-light mb-4">My Coupons</h1>

    <v-alert
      v-for="p in pending" :key="p.signature" type="warning" variant="tonal" class="mb-4"
      title="Payment not registered yet"
    >
      You paid {{ formatSol(p.lamports) }} for {{ (p.combinations || p.coupons?.length || 0).toLocaleString() }} combination(s) in {{ p.eventTitle }},
      but the coupons were not registered.
      <a :href="explorerTx(p.signature)" target="_blank" rel="noopener">View payment</a>
      <div v-if="expired(p)" class="mt-2">
        This payment is more than an hour old and can no longer be registered automatically.
        Send the transaction link to the admin for a refund.
      </div>
      <template #append>
        <div class="d-flex flex-column ga-2">
          <v-btn v-if="!expired(p)" color="warning" variant="flat" :loading="retrying === p.signature" @click="retry(p)">Retry</v-btn>
          <v-btn v-else variant="text" size="small" @click="dismiss(p)">Dismiss</v-btn>
        </div>
      </template>
    </v-alert>

    <v-card v-if="totalWinnings" color="success" variant="tonal" class="mb-4">
      <v-card-text class="text-h6">
        <v-icon class="mr-2">mdi-trophy</v-icon>
        Total winnings: {{ formatSol(totalWinnings) }}
        <div class="text-body-2 mt-1">Winnings are paid to your wallet by the admin from the devnet treasury.</div>
      </v-card-text>
    </v-card>

    <v-progress-linear v-if="loading" indeterminate class="mb-4" />
    <v-card v-if="!loading && entries.length === 0" variant="outlined" class="pa-6 text-center">
      <p class="text-medium-emphasis mb-4">You have not placed any coupons yet.</p>
      <v-btn color="primary" to="/">Browse events</v-btn>
    </v-card>

    <v-card v-for="entry in entries" :key="entry.id" class="mb-4">
      <v-card-title class="d-flex align-center flex-wrap ga-2">
        <router-link :to="`/events/${entry.eventId}`" class="text-decoration-none">{{ entry.eventTitle || 'Event' }}</router-link>
        <v-chip v-if="entry.event" :color="statusColors[entry.event.status]" size="small" variant="flat">{{ entry.event.status }}</v-chip>
        <v-spacer />
        <v-chip v-if="entry.results" :color="entry.winningsLamports ? 'success' : 'default'" variant="flat">
          {{ entry.winningsLamports ? `Won ${formatSol(entry.winningsLamports)}` : 'No win' }}
        </v-chip>
      </v-card-title>
      <v-card-subtitle class="text-wrap">
        {{ (entry.combinations || entry.coupons?.length || 0).toLocaleString() }} combination(s) · paid {{ formatSol(entry.lamports) }} · {{ formatDate(entry.createdAt) }} ·
        <a :href="explorerTx(entry.signature)" target="_blank" rel="noopener">payment</a>
      </v-card-subtitle>
      <v-card-text>
        <template v-if="entry.system?.length">
          <div v-for="(picks, index) in entry.system" :key="index" class="d-flex align-center flex-wrap ga-2 py-1">
            <span class="game-label">{{ index + 1 }}. {{ gameLabel(entry, index) }}</span>
            <v-chip v-for="pick in picks" :key="pick" size="small" label variant="flat" color="primary">{{ pick }}</v-chip>
          </div>
          <div v-if="entry.scores?.length" class="mt-3">
            <div v-for="band in entry.scores" :key="band.correct" class="py-1">
              {{ band.combinations.toLocaleString() }} with {{ band.correct }} correct
              <strong v-if="band.winningsLamports" class="text-success ml-2">{{ formatSol(band.winningsLamports) }}</strong>
            </div>
          </div>
          <div v-else class="text-caption text-medium-emphasis mt-2">Waiting for results</div>
        </template>
        <template v-else>
          <div v-for="(coupon, i) in entry.coupons" :key="i" class="d-flex align-center flex-wrap ga-3 py-2">
            <span class="text-medium-emphasis" style="min-width: 28px">#{{ i + 1 }}</span>
            <CouponPicks :coupon="coupon" :games="entry.event?.games ?? []" />
            <template v-if="entry.results">
              <span>{{ entry.results[i].correct }} / {{ coupon.length }} correct</span>
              <strong v-if="entry.results[i].winningsLamports" class="text-success">
                {{ formatSol(entry.results[i].winningsLamports) }}
              </strong>
            </template>
            <span v-else class="text-caption text-medium-emphasis">Waiting for results</span>
          </div>
        </template>
      </v-card-text>
    </v-card>
  </v-container>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import CouponPicks from '@/components/CouponPicks.vue'
import { entryApi } from '@/services/api'
import { PAYMENT_MAX_AGE_MS, loadPending, removePending, submitPending } from '@/services/entry'
import { auth } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { explorerTx, formatDate, formatSol, statusColors } from '@/utils/format'

const entries = ref([])
const pending = ref([])
const loading = ref(false)
const retrying = ref(null)

const totalWinnings = computed(() => entries.value.reduce((sum, e) => sum + e.winningsLamports, 0))
const gameLabel = (entry, index) => {
  const game = entry.event?.games?.[index]
  return game ? `${game.homeTeam} vs ${game.awayTeam}` : 'Game'
}
const expired = p => Date.now() - p.paidAt > PAYMENT_MAX_AGE_MS

async function load () {
  pending.value = loadPending(auth.wallet)
  loading.value = true
  try {
    entries.value = await entryApi.mine()
    const registered = new Set(entries.value.map(e => e.signature))
    for (const p of pending.value.filter(p => registered.has(p.signature))) removePending(auth.wallet, p.signature)
    pending.value = loadPending(auth.wallet)
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

async function retry (p) {
  retrying.value = p.signature
  try {
    await submitPending(auth.wallet, p)
    notify('Coupons registered')
  } catch (error) {
    notifyError(error)
    if (/already been used/.test(error.message)) removePending(auth.wallet, p.signature)
  } finally {
    retrying.value = null
    await load()
  }
}

function dismiss (p) {
  removePending(auth.wallet, p.signature)
  pending.value = loadPending(auth.wallet)
}

onMounted(load)
</script>

<style scoped>
.game-label { min-width: 180px; }
</style>
