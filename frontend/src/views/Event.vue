<template>
  <v-container class="py-6" max-width="1100">
    <v-btn variant="text" prepend-icon="mdi-arrow-left" to="/" class="mb-2">Events</v-btn>
    <v-progress-linear v-if="loading && !event" indeterminate />

    <template v-if="event">
      <v-card class="mb-4">
        <v-card-title class="d-flex align-center flex-wrap ga-2">
          {{ event.title }}
          <v-chip :color="statusColors[event.status]" size="small" variant="flat">{{ event.status }}</v-chip>
          <v-chip v-if="event.rolloverInLamports" color="amber" size="small" variant="flat" prepend-icon="mdi-trophy">
            Jackpot +{{ formatSol(event.rolloverInLamports) }}
          </v-chip>
        </v-card-title>
        <v-card-subtitle class="text-wrap">
          {{ event.gameCount }} games · {{ formatSol(event.couponPriceLamports) }} per coupon ·
          entries close {{ formatDate(event.startTime) }}
        </v-card-subtitle>
        <v-card-text v-if="auth.isAdmin && event.status !== 'completed'" class="pt-2 pb-0">
          <v-btn size="small" variant="tonal" prepend-icon="mdi-refresh" :loading="refreshingScores" @click="refreshScores">
            Refresh scores
          </v-btn>
          <div class="text-caption text-medium-emphasis mt-1">
            Scores refresh on their own 45 and 90 minutes after entries close. This button reads them now.
          </div>
        </v-card-text>
        <v-card-text>
          <p v-if="event.description" class="mb-4">{{ event.description }}</p>
          <p v-if="event.chain" class="text-body-2">
            Entries are held by the on-chain vault below. Reveal your picks in My Coupons after entries close; a coupon that
            is not revealed before {{ formatDate(event.endTime) }} cannot win and its stake stays in the prize pool.
          </p>
          <template v-if="!event.chain">
            <v-table density="compact">
              <thead>
                <tr>
                  <th>Correct picks</th><th>Share</th><th>{{ settlement ? 'Pot' : 'Pot so far' }}</th>
                  <th v-if="settlement">Winning coupons</th><th v-if="settlement">Each wins</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="tier in tierRows" :key="tier.correct">
                  <td>{{ tier.correct }} of {{ event.gameCount }}</td>
                  <td>{{ tier.percent }}%</td>
                  <td>{{ formatSol(tier.potLamports) }}</td>
                  <template v-if="settlement">
                    <td>{{ tier.winners }}</td>
                    <td>{{ tier.winners ? formatSol(tier.perWinnerLamports) : 'rolls over' }}</td>
                  </template>
                </tr>
              </tbody>
            </v-table>
            <p class="text-caption text-medium-emphasis mt-2">
              {{ event.couponsSold }} coupon{{ event.couponsSold === 1 ? '' : 's' }} sold. The house keeps {{ houseCut }}% of ticket sales.
              A prize tier without winners rolls over to the jackpot (all games correct) of the next event.
            </p>
            <p v-if="settlement?.rolloverOutLamports" class="text-caption mt-1">
              {{ formatSol(settlement.rolloverOutLamports) }} rolled over to
              <router-link v-if="settlement.rolloverToEventId" :to="`/events/${settlement.rolloverToEventId}`">the next event</router-link>
              <span v-else>the next event that is created</span>.
            </p>
          </template>
        </v-card-text>
      </v-card>

      <VaultPanel v-if="event.chain" :event="event" class="mb-4" @changed="refreshEvent" />

      <v-card v-if="event.status !== 'open'" class="mb-4" title="Games">
        <v-card-subtitle v-if="event.status !== 'completed'" class="text-wrap">
          A live score is what the match shows now. It is not the settled result.
        </v-card-subtitle>
        <v-table density="compact">
          <thead><tr><th>#</th><th>Game</th><th>Kickoff</th><th>Result</th></tr></thead>
          <tbody>
            <tr v-for="(game, index) in event.games" :key="index">
              <td>{{ index + 1 }}</td>
              <td>{{ game.homeTeam }} vs {{ game.awayTeam }}</td>
              <td>{{ formatDate(game.kickoff) }}</td>
              <td>
                <template v-if="game.result">
                  <strong>{{ game.result === 'C' ? 'Void' : game.result }}</strong>
                  <v-chip v-if="game.result === 'C'" size="x-small" color="warning" class="ml-2">counts for every pick</v-chip>
                  <v-chip v-else-if="game.cancelled" size="x-small" color="warning" class="ml-2">cancelled, drawn</v-chip>
                  <div v-if="game.draw" class="text-caption text-medium-emphasis">
                    Weights 1/X/2: {{ game.draw.weights.join(' / ') }} ·
                    <a :href="explorerBlock(game.draw.slot)" target="_blank" rel="noopener">block {{ game.draw.slot }}</a>
                  </div>
                </template>
                <MatchScore v-else :game="projection?.games?.[index]" :kickoff="game.kickoff" />
              </td>
            </tr>
          </tbody>
        </v-table>
        <v-card-actions v-if="event.status === 'completed'">
          <v-btn color="primary" prepend-icon="mdi-ticket" to="/coupons">See my coupons</v-btn>
        </v-card-actions>
      </v-card>

      <v-card v-if="event.status === 'open'">
        <v-card-title>Your coupon</v-card-title>
        <v-card-subtitle class="text-wrap">
          Select one or more of 1 (home win), X (draw) and 2 (away win) for every game.
          Every combination becomes its own paid row.
        </v-card-subtitle>
        <v-card-text>
          <v-alert type="info" variant="tonal" class="mb-4">
            Example: 1 and X on one game, and only 2 on the others, makes 2 combinations.
            Selecting two outcomes on two games makes 4.
          </v-alert>
          <v-alert v-if="isTreasury" type="warning" variant="tonal" class="mb-4">
            This wallet is the treasury wallet and cannot pay itself. Sign in with another wallet to play.
          </v-alert>
          <div class="d-flex flex-wrap ga-2 mb-4">
            <v-btn size="small" color="success" variant="tonal" prepend-icon="mdi-home" :disabled="paying" @click="selectAll('1')">All 1</v-btn>
            <v-btn size="small" color="warning" variant="tonal" prepend-icon="mdi-minus" :disabled="paying" @click="selectAll('X')">All X</v-btn>
            <v-btn size="small" color="info" variant="tonal" prepend-icon="mdi-arrow-right" :disabled="paying" @click="selectAll('2')">All 2</v-btn>
            <v-btn size="small" color="error" variant="tonal" prepend-icon="mdi-close" :disabled="paying" @click="clearPicks">Clear</v-btn>
          </div>
          <div v-for="(game, g) in event.games" :key="g" class="d-flex align-center flex-wrap ga-3 py-2 game-row">
            <div class="game-cell">
              <div class="font-weight-medium">{{ g + 1 }}. {{ game.homeTeam }} vs {{ game.awayTeam }}</div>
              <MatchScore :game="projection?.games?.[g]" :kickoff="game.kickoff" />
            </div>
            <v-btn
              v-for="o in outcomes" :key="o.value" :title="o.label" min-width="44" :disabled="paying"
              :color="isSelected(g, o.value) ? 'primary' : undefined"
              :variant="isSelected(g, o.value) ? 'flat' : 'outlined'"
              @click="togglePick(g, o.value)"
            >
              {{ o.value }}
            </v-btn>
          </div>
          <div v-if="preview.length" class="mt-4">
            <div class="text-subtitle-2 mb-2">Combinations</div>
            <div class="d-flex flex-column ga-2">
              <CouponPicks v-for="coupon in preview" :key="coupon" :coupon="coupon" :games="event.games" />
            </div>
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="flex-wrap ga-2 pa-4">
          <div>
            <div class="text-subtitle-1">
              {{ combinationTotal.toLocaleString() }} combination{{ combinationTotal === 1 ? '' : 's' }} × {{ formatSol(event.couponPriceLamports) }} =
              <strong>{{ formatSol(totalLamports) }}</strong>
            </div>
            <div class="text-caption text-medium-emphasis">
              {{ statusText }}
            </div>
            <div class="text-caption text-medium-emphasis mt-1">
              You pay the coupon price only. A losing coupon keeps its stake in the pool. There is no account deposit and no monthly draw.
            </div>
            <ProjectedPayout :system="systemRows(picks)" :projection="projection" />
            <div v-if="lowBalance" class="text-caption text-error">Your devnet balance looks too low for this payment.</div>
          </div>
          <v-spacer />
          <v-btn
            color="primary" variant="flat" size="large" prepend-icon="mdi-wallet"
            :disabled="!complete || isTreasury" :loading="paying" @click="pay"
          >
            Pay with Phantom
          </v-btn>
        </v-card-actions>
        <v-alert v-if="paying" type="info" variant="tonal" class="ma-4 mt-0">{{ stepLabels[step] }}</v-alert>
      </v-card>
    </template>
  </v-container>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CouponPicks from '@/components/CouponPicks.vue'
import MatchScore from '@/components/MatchScore.vue'
import ProjectedPayout from '@/components/ProjectedPayout.vue'
import VaultPanel from '@/components/VaultPanel.vue'
import { eventApi } from '@/services/api'
import { payAndEnter, placeOnChain } from '@/services/entry'
import { auth, loadConfig, refreshBalance } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { combinationCount, expandCombinations, systemRows } from '@/utils/coupons'
import { explorerBlock, formatDate, formatSol, outcomes, statusColors } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const event = ref(null)
const projection = ref(null)
const loading = ref(false)
const refreshingScores = ref(false)
const picks = ref([])
const paying = ref(false)
const step = ref('sign')

const stepLabels = computed(() => event.value?.chain ? {
  sign: 'Approve the entry in Phantom.',
  confirm: 'Waiting for the entry to be finalized on devnet...',
  schedule: 'Saving the time-locked picks. They are published when entries close.'
} : {
  sign: 'Approve the payment in Phantom (make sure Phantom is set to Devnet)...',
  confirm: 'Waiting for the payment to be finalized on devnet. This can take about a minute...',
  submit: 'Payment confirmed. Registering your coupons...'
})

const maxCombinations = computed(() => auth.config?.maxCombinations ?? auth.config?.maxCouponsPerEntry ?? 44000)
const houseCut = computed(() => auth.config?.houseCutPercent ?? 5)
const isTreasury = computed(() => auth.config && auth.wallet === auth.config.treasuryWallet)
const settlement = computed(() => event.value?.settlement)
const combinationTotal = computed(() => combinationCount(picks.value))
const preview = computed(() => expandCombinations(picks.value))
const missingGames = computed(() => picks.value.filter(game => game.length === 0).length)
const tooMany = computed(() => combinationTotal.value > maxCombinations.value)
const totalLamports = computed(() => combinationTotal.value * (event.value?.couponPriceLamports ?? 0))
const complete = computed(() => missingGames.value === 0 && combinationTotal.value > 0 && !tooMany.value)
const lowBalance = computed(() => auth.balance !== null && auth.balance * 1e9 < totalLamports.value + 10000)
const statusText = computed(() => {
  if (missingGames.value) return `${missingGames.value} game${missingGames.value === 1 ? '' : 's'} still need a pick`
  if (tooMany.value) return `At most ${maxCombinations.value.toLocaleString()} combinations per payment. Remove some extra picks.`
  return 'Plus a small Solana network fee, paid from your wallet.'
})

const tierRows = computed(() => {
  const e = event.value
  if (settlement.value) return settlement.value.tiers
  const sales = e.couponsSold * e.couponPriceLamports
  return e.tiers.map(t => ({
    ...t,
    potLamports: Math.floor(sales * t.percent / 100) + (t.correct === e.gameCount ? e.rolloverInLamports : 0)
  }))
})

async function refreshScores () {
  refreshingScores.value = true
  try {
    projection.value = await eventApi.refreshScores(event.value.id)
    notify('Scores refreshed')
  } catch (error) {
    notifyError(error)
  } finally {
    refreshingScores.value = false
  }
}

async function loadProjection () {
  if (!event.value || event.value.status === 'completed') {
    projection.value = null
    return
  }
  try {
    projection.value = await eventApi.projection(event.value.id)
  } catch {
    projection.value = null
  }
}

function resetPicks () {
  picks.value = event.value.games.map(() => [])
}

function isSelected (game, outcome) {
  return picks.value[game]?.includes(outcome)
}

function togglePick (game, outcome) {
  const selected = picks.value[game]
  picks.value[game] = selected.includes(outcome)
    ? selected.filter(value => value !== outcome)
    : [...selected, outcome]
}

function selectAll (outcome) {
  picks.value = picks.value.map(() => [outcome])
}

function clearPicks () {
  resetPicks()
}

async function load () {
  loading.value = true
  try {
    await loadConfig()
    event.value = await eventApi.get(route.params.id)
    if (event.value.status === 'open') resetPicks()
  } catch (error) {
    notifyError(error)
    router.push('/')
  } finally {
    loading.value = false
  }
}

async function refreshEvent () {
  try {
    const fresh = await eventApi.get(route.params.id)
    const wasOpen = event.value?.status === 'open'
    event.value = fresh
    if (!wasOpen && fresh.status === 'open') resetPicks()
  } catch (error) {
    notifyError(error)
  }
}

async function pay () {
  paying.value = true
  try {
    const entry = {
      config: auth.config,
      wallet: auth.wallet,
      event: event.value,
      system: systemRows(picks.value),
      combinations: combinationTotal.value,
      onStep: s => { step.value = s }
    }
    if (event.value.chain) {
      const placed = await placeOnChain(entry)
      notify(placed.armError
        ? `Coupon entered. The automatic reveal was not saved (${placed.armError}). Arm it from My Coupons.`
        : 'Coupon entered. Your picks are published automatically when entries close.')
    } else {
      await payAndEnter(entry)
      notify('Coupons placed. Good luck!')
    }
    refreshBalance()
    router.push('/coupons')
  } catch (error) {
    if (step.value === 'sign' || /No SOL was sent|failed on chain/.test(error.message)) notifyError(error)
    else notify(`${error.message} Your payment is saved; retry it from My Coupons.`, 'warning')
  } finally {
    paying.value = false
  }
}

watch(() => route.params.id, id => {
  if (!id) return
  picks.value = []
  load().then(loadProjection)
}, { immediate: true })

const projectionTimer = setInterval(loadProjection, 60000)
onBeforeUnmount(() => clearInterval(projectionTimer))
</script>

<style scoped>
.game-row { border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); }
.game-cell { flex: 1 1 220px; }
</style>
