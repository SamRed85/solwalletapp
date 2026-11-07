<template>
  <v-container class="py-8" max-width="900">
    <div class="text-center mb-6 home-hero">
      <v-icon :size="mobile ? 48 : 72" color="primary" class="mb-3">mdi-target</v-icon>
      <h1 class="text-h3 mb-2">Welcome to Sol</h1>
      <p class="text-h6 text-medium-emphasis">Predict match outcomes with your Solana wallet</p>
    </div>

    <v-card>
        <v-card-title class="d-flex align-center flex-wrap ga-2">
          <v-icon class="mr-2">mdi-calendar</v-icon>
          Events
          <v-spacer />
          <v-btn v-if="auth.isAdmin" variant="text" color="warning" prepend-icon="mdi-shield-crown" to="/admin">
            Manage
          </v-btn>
        </v-card-title>
        <v-progress-linear v-if="loading" indeterminate />
        <v-card-text v-if="!loading && events.length === 0" class="text-center text-medium-emphasis">
          No events yet.
        </v-card-text>
        <v-list v-else lines="three">
          <v-list-item
            v-for="event in events" :key="event.id" class="event-link" :to="`/events/${event.id}`"
            :disabled="event.status === 'draft' && !auth.isAdmin"
          >
            <v-list-item-title class="d-flex align-center ga-1">
              <span class="event-name">{{ event.title }}</span>
              <v-icon v-if="jackpotOf(event)" color="amber" size="small" title="Jackpot event">mdi-trophy</v-icon>
            </v-list-item-title>
            <v-list-item-subtitle>
              {{ event.gameCount }} games · {{ formatSol(event.couponPriceLamports) }} per coupon · entries close {{ formatDate(event.startTime) }}
            </v-list-item-subtitle>
            <div class="d-flex flex-wrap ga-2 mt-2">
              <v-chip v-if="jackpotOf(event)" color="amber" size="small" variant="flat" prepend-icon="mdi-trophy">
                Jackpot {{ formatSol(jackpotOf(event)) }}
              </v-chip>
              <v-chip :color="statusColors[event.status]" size="small" variant="flat">
                {{ event.status === 'open' ? 'play' : event.status }}
              </v-chip>
            </div>
          </v-list-item>
        </v-list>
      </v-card>

      <v-card v-if="!auth.wallet" variant="outlined" class="pa-6 text-center mt-4">
        <template v-if="phantomInstalled">
          <h2 class="text-h6 mb-2">Play when you are ready</h2>
          <p class="text-body-1 text-medium-emphasis mb-4">
            You can look through the events without a wallet. Connect Phantom and sign a message when you want to enter a coupon. Signing is free and sends no transaction.
          </p>
          <v-btn color="primary" size="large" prepend-icon="mdi-wallet" :loading="auth.busy" @click="onLogin">
            Connect wallet
          </v-btn>
        </template>
        <template v-else>
          <h2 class="text-h6 mb-2">Phantom wallet required to play</h2>
          <p class="text-body-1 text-medium-emphasis mb-4">
            Install the Phantom browser extension and switch it to Devnet in its settings. You can still browse the events first.
          </p>
          <v-btn color="primary" href="https://phantom.app/" target="_blank" prepend-icon="mdi-download">
            Install Phantom
          </v-btn>
        </template>
      </v-card>

      <div v-else class="text-center mt-4">
        <v-btn variant="text" prepend-icon="mdi-ticket" to="/coupons">My Coupons</v-btn>
      </div>
  </v-container>
</template>

<script setup>
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import { eventApi } from '@/services/api'
import { eventPda, fetchEvent } from '@/services/program'
import { isPhantomInstalled } from '@/services/solana'
import { auth, loadConfig, login } from '@/stores/auth'
import { notifyError } from '@/stores/notify'
import { formatDate, formatSol, statusColors } from '@/utils/format'

const { smAndDown: mobile } = useDisplay()
const phantomInstalled = ref(isPhantomInstalled())
const events = ref([])
const loading = ref(false)
const chainJackpots = ref({})

// A vault event keeps its carried-over jackpot on chain only.
function jackpotOf (event) {
  if (event.chain) return chainJackpots.value[event.id] || 0
  return event.rolloverInLamports || 0
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
        if (state.rolloverIn > 0n) found[event.id] = Number(state.rolloverIn)
      } catch {
        // An unreadable vault shows no jackpot badge.
      }
    }))
    chainJackpots.value = found
  } catch {
    chainJackpots.value = {}
  }
}

const newestFirst = (a, b) => new Date(b.createdAt || b.startTime) - new Date(a.createdAt || a.startTime)
let phantomTimer

onMounted(() => {
  loadEvents()
  phantomTimer = setInterval(() => {
    phantomInstalled.value = isPhantomInstalled()
    if (phantomInstalled.value) clearInterval(phantomTimer)
  }, 250)
})
onUnmounted(() => clearInterval(phantomTimer))

async function loadEvents () {
  loading.value = true
  try {
    events.value = (await eventApi.list()).sort(newestFirst)
    loadChainJackpots(events.value)
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

async function onLogin () {
  try {
    await login()
  } catch (error) {
    notifyError(error)
  }
}

watch(() => auth.wallet, () => loadEvents())
</script>

<style scoped>
.event-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.event-link {
  transition: transform 180ms ease, box-shadow 180ms ease;
}

.event-link:not(.v-list-item--disabled):hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-surface);
}

.event-link:not(.v-list-item--disabled):active {
  transform: translateY(0);
}
</style>
