<template>
  <v-container class="py-6" max-width="1100">
    <v-btn variant="text" prepend-icon="mdi-arrow-left" to="/" class="mb-2">Events</v-btn>
    <v-progress-linear v-if="loading && !event" indeterminate />

    <template v-if="event">
      <v-card class="mb-4">
        <v-card-title class="d-flex align-center flex-wrap ga-2">
          <span class="event-title">{{ event.title }}</span>
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
            Entries are held by the on-chain vault. Reveal your picks in My Coupons after entries close; a coupon that
            is not revealed before {{ formatDate(event.endTime) }} cannot win and its stake stays in the prize pool.
          </p>
          <template v-if="!event.chain">
            <div class="sales-block mb-2">
              <div class="sales-label">Pot</div>
              <div class="sales-amount">{{ formatSol(salesLamports) }}</div>
            </div>
            <p class="text-caption text-medium-emphasis mb-3">
              {{ event.couponsSold }} coupon{{ event.couponsSold === 1 ? '' : 's' }} sold. The house keeps {{ houseCut }}% of ticket sales.
            </p>
            <v-expansion-panels>
              <v-expansion-panel>
                <v-expansion-panel-title>{{ settlement ? 'Winnings' : 'Pot so far' }}</v-expansion-panel-title>
                <v-expansion-panel-text>
                  <v-table density="compact">
                    <thead>
                      <tr>
                        <th>Correct picks</th><th>Share</th><th>{{ settlement ? 'Pot' : 'Pot so far' }}</th>
                        <th v-if="settlement">Winning coupons</th><th v-if="settlement">Each wins</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="tier in tierRows" :key="tier.correct">
                        <td>{{ tier.correct }}</td>
                        <td>{{ tier.percent }}%</td>
                        <td>{{ formatSol(tier.potLamports) }}</td>
                        <template v-if="settlement">
                          <td>{{ tier.winners }}</td>
                          <td>{{ tier.perWinnerLamports ? formatSol(tier.perWinnerLamports) : 'rolls over' }}</td>
                        </template>
                      </tr>
                    </tbody>
                  </v-table>
                  <p class="text-caption text-medium-emphasis mt-2 mb-0">
                    A prize tier rolls over to the jackpot (all games correct) of the next event when nobody hits it, or when each winning row would receive less than 0.01 SOL.
                  </p>
                  <p v-if="settlement?.rolloverOutLamports" class="text-caption mt-1 mb-0">
                    {{ formatSol(settlement.rolloverOutLamports) }} rolled over to
                    <router-link v-if="settlement.rolloverToEventId" :to="`/events/${settlement.rolloverToEventId}`">the next event</router-link>
                    <span v-else>the next event that is created</span>.
                  </p>
                </v-expansion-panel-text>
              </v-expansion-panel>
            </v-expansion-panels>
          </template>
        </v-card-text>
        <VaultPanel v-if="event.chain" embedded :event="event" @changed="refreshEvent" />
      </v-card>

      <v-card v-if="event.status !== 'open'" class="mb-4" title="Games">
        <v-card-subtitle v-if="event.status !== 'completed'" class="text-wrap">
          A live score is what the match shows now. It is not the settled result.
        </v-card-subtitle>
        <div v-for="(game, index) in event.games" :key="index" class="game-row">
          <div class="game-top">
            <div class="game-name-line">
              <div class="font-weight-medium"><span class="game-no">{{ index + 1 }}.</span>{{ game.homeTeam }} vs {{ game.awayTeam }}</div>
              <MatchScore
                v-if="liveGame(index)?.linked"
                :game="liveGame(index)"
                :kickoff="game.kickoff"
                :placeholder="!game.result"
                :finished="game.result === '1' || game.result === 'X' || game.result === '2'"
              />
            </div>
            <span class="marks">
              <span v-for="mark in marksFor(game)" :key="mark" class="mark" :class="{ on: mark === outcomeAt(index) }">{{ mark }}</span>
            </span>
          </div>
          <div class="game-when">{{ kickoffStamp(game.kickoff) }}</div>
          <div v-if="game.draw" class="text-caption text-medium-emphasis">
            Weights 1/X/2: {{ game.draw.weights.join(' / ') }} ·
            <a :href="explorerBlock(game.draw.slot)" target="_blank" rel="noopener">block {{ game.draw.slot }}</a>
          </div>
        </div>
        <v-card-text v-if="officialResults || projection?.revealClosed">
          <div class="text-subtitle-2">All rows</div>
          <div v-if="!allRows.length" class="text-caption text-medium-emphasis">
            The totals appear once every match has a score.
          </div>
          <template v-else>
            <div class="text-caption text-medium-emphasis mb-1">
              <template v-if="officialResults">Every revealed row in this event.</template>
              <template v-else>If these scores stand. Counted from revealed coupons.</template>
            </div>
            <v-table>
              <thead><tr><th>Correct picks</th><th>Rows</th><th>Payout</th></tr></thead>
              <tbody>
                <tr v-for="row in allRows" :key="row.correct">
                  <td>{{ row.correct }}</td>
                  <td>{{ Number(row.rows).toLocaleString() }}</td>
                  <td>{{ row.each ? formatSol(row.each) : 'rolls over' }}</td>
                </tr>
              </tbody>
            </v-table>
          </template>
        </v-card-text>
        <v-card-actions v-if="event.status === 'completed' && auth.wallet">
          <v-btn color="primary" prepend-icon="mdi-ticket" to="/coupons">See my coupons</v-btn>
        </v-card-actions>
      </v-card>

      <v-card v-if="event.status === 'open'" class="coupon-entry">
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
          <div v-if="event.games.length" class="game-row fill-row">
            <div class="game-top">
              <div class="fill-spacer">
                <v-btn class="pick-btn" variant="text" :disabled="paying" title="Clear every pick" @click="clearPicks">
                  <v-icon icon="mdi-close" size="16" />
                </v-btn>
              </div>
              <v-btn
                v-for="o in outcomes" :key="o.value" class="pick-btn" variant="text" :disabled="paying"
                :title="`${o.label} on every game`" @click="selectAll(o.value)"
              >
                <v-icon icon="mdi-arrow-down" size="16" />
              </v-btn>
            </div>
          </div>
          <div v-for="(game, g) in event.games" :key="g" class="game-row">
            <div class="game-top">
              <div class="game-name-line">
                <div class="font-weight-medium"><span class="game-no">{{ g + 1 }}.</span>{{ game.homeTeam }} vs {{ game.awayTeam }}</div>
                <MatchScore v-if="projection?.games?.[g]?.linked" :game="projection.games[g]" :kickoff="game.kickoff" />
              </div>
              <v-btn
                v-for="o in outcomes" :key="o.value" class="pick-btn" :title="o.label" :disabled="paying"
                :color="isSelected(g, o.value) ? 'primary' : undefined"
                :variant="isSelected(g, o.value) ? 'flat' : 'outlined'"
                @click="togglePick(g, o.value)"
              >
                {{ o.value }}
              </v-btn>
            </div>
            <div class="game-when">{{ kickoffStamp(game.kickoff) }}</div>
          </div>
          <div v-if="preview.length" class="mt-4">
            <div class="text-subtitle-2 mb-2">Combinations</div>
            <div class="d-flex flex-column ga-2">
              <CouponPicks v-for="coupon in preview" :key="coupon" :coupon="coupon" :games="event.games" />
            </div>
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pay-bar flex-wrap ga-2 pa-4">
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
            v-if="!auth.wallet"
            color="primary" variant="flat" size="large" prepend-icon="mdi-wallet"
            :loading="auth.busy" @click="onLogin"
          >
            Connect wallet to play
          </v-btn>
          <v-btn
            v-else
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
import { auth, loadConfig, login, refreshBalance } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { combinationCount, expandCombinations, systemRows } from '@/utils/coupons'
import { kickoffStamp } from '@/services/projection'
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

const salesLamports = computed(() => {
  const e = event.value
  if (!e) return 0
  return e.couponsSold * e.couponPriceLamports
})

const tierRows = computed(() => {
  const e = event.value
  if (settlement.value) return settlement.value.tiers
  const sales = salesLamports.value
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

function liveGame (index) {
  return projection.value?.games?.[index] || null
}

function outcomeAt (index) {
  const official = event.value?.games?.[index]?.result
  if (official === '1' || official === 'X' || official === '2' || official === 'C') return official
  return liveGame(index)?.projected || ''
}

function marksFor (game) {
  return game.result === 'C' ? ['C'] : ['1', 'X', '2']
}

const officialResults = computed(() => {
  const games = event.value?.games
  if (!games?.length || games.some(game => !game.result || !'1X2C'.includes(game.result))) return ''
  return games.map(game => game.result).join('')
})

const allRows = computed(() => {
  const rows = projection.value?.rows || []
  const paying = new Set((event.value?.tiers || []).map(tier => tier.correct))
  if (!paying.size) return rows
  return rows.filter(row => paying.has(row.correct))
})

async function onLogin () {
  try {
    await login()
  } catch (error) {
    notifyError(error)
  }
}

async function loadProjection () {
  if (!auth.wallet || !event.value?.games?.some(game => game.fixtureId)) {
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
  picks.value = picks.value.map(selected => selected.includes(outcome) ? selected : [...selected, outcome])
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

watch(() => auth.wallet, wallet => {
  if (wallet && event.value) loadProjection()
})

const projectionTimer = setInterval(loadProjection, 60000)
onBeforeUnmount(() => clearInterval(projectionTimer))
</script>

<style scoped>
.event-title {
  font-size: 3.4rem;
  font-weight: 600;
  line-height: 1.05;
  letter-spacing: -0.02em;
}
.game-row {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 1px;
  padding: 8px 0;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.game-top {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
}
.fill-row { border-bottom: 0; padding-bottom: 0; }
.fill-spacer { display: flex; justify-content: flex-end; min-width: 0; }
.coupon-entry .game-top {
  display: grid;
  grid-template-columns: minmax(0, 1fr) repeat(3, 25px);
  column-gap: 8px;
  align-items: center;
}
.game-no { margin-right: 3px; }
.game-name-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  max-width: 100%;
  min-width: 0;
}
.game-name-line .font-weight-medium {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.game-name-line :deep(.when) { display: none; }
.game-name-line :deep(.match-score) { flex: none; gap: 4px; }
.game-name-line :deep(.clock) { min-width: 0; font-size: 11px; }
.game-name-line :deep(.pill) { min-width: 0; padding: 1px 6px; font-size: 11px; }
.game-when {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  line-height: 1.1;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.62);
  white-space: nowrap;
}
.pick-btn {
  flex: 0 0 25px;
  width: 25px !important;
  min-width: 25px !important;
  height: 25px !important;
  min-height: 25px !important;
  padding: 0 !important;
  font-size: 12px;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease, transform 120ms ease;
}
.pick-btn:active {
  transform: scale(0.98);
}
.marks { display: inline-flex; gap: 8px; flex: none; }
.mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 25px;
  min-width: 25px;
  height: 25px;
  padding: 0;
  border-radius: 6px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.16);
  background: transparent;
  color: rgba(var(--v-theme-on-surface), 0.45);
  font-weight: 700;
  font-size: 12px;
}
.mark.on {
  border-color: transparent;
  background: rgba(var(--v-theme-on-surface), 0.14);
  color: rgb(var(--v-theme-on-surface));
}
@media (max-width: 959px) {
  .event-title { font-size: 2rem; }
  .game-row { flex-wrap: nowrap; width: 100%; }
  .game-name-line .font-weight-medium { font-size: 10px; }
  .coupon-entry .game-name-line .font-weight-medium { font-size: 14px; }
  .game-when { font-size: 9px; }
  .pay-bar { flex-direction: column; align-items: stretch; }
  .pay-bar .v-btn { width: 100%; }
}
</style>
