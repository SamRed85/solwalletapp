<template>
  <v-container class="py-6" max-width="1100">
    <v-btn variant="text" prepend-icon="mdi-arrow-left" to="/" class="mb-2">Events</v-btn>
    <v-progress-linear v-if="loading && !event" indeterminate />

    <template v-if="event">
      <v-card class="mb-4">
        <v-card-title class="d-flex align-center flex-wrap ga-2">
          <span class="event-title">{{ event.title }}</span>
          <v-icon v-if="jackpotLamports" color="amber" :size="28" title="Jackpot event">mdi-trophy</v-icon>
          <v-chip :color="statusColors[event.status]" size="small" variant="flat">{{ event.status }}</v-chip>
          <v-chip v-if="jackpotLamports" color="amber" size="small" variant="flat" prepend-icon="mdi-trophy">
            Jackpot {{ formatSol(jackpotLamports) }}
          </v-chip>
        </v-card-title>
        <v-card-subtitle class="text-wrap">
          {{ event.gameCount }} games · {{ event.league ? `${event.couponPriceLamports} RAD` : formatSol(event.couponPriceLamports) }} per coupon ·
          at least {{ limits.minRows.toLocaleString() }} row{{ limits.minRows === 1 ? '' : 's' }} ·
          {{ limits.count.toLocaleString() }} of {{ limits.maxCoupons.toLocaleString() }} coupons ·
          entries close {{ formatDate(event.startTime) }}
        </v-card-subtitle>
        <v-card-text v-if="auth.isAdmin && event.status !== 'completed'" class="pt-2 pb-0">
          <div class="d-flex flex-wrap ga-2">
            <v-btn size="small" variant="tonal" prepend-icon="mdi-refresh" :loading="refreshingScores" @click="refreshScores">
              Refresh scores
            </v-btn>
            <v-btn size="small" variant="tonal" prepend-icon="mdi-percent" :loading="fetchingOdds" @click="fetchOdds">
              Fetch odds
            </v-btn>
          </div>
          <div class="text-caption text-medium-emphasis mt-1">
            Scores refresh on their own 45 and 90 minutes after entries close. This button reads them now.
            Fetch odds reads the Match Winner line once and saves it.
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
              <div class="sales-label">Total pot</div>
              <div class="sales-amount">{{ formatSol(salesLamports + jackpotLamports) }}</div>
            </div>
            <p class="text-caption text-medium-emphasis mb-3">
              {{ formatSol(salesLamports) }} from {{ event.couponsSold }} coupon{{ event.couponsSold === 1 ? '' : 's' }}<template v-if="jackpotLamports"> + {{ formatSol(jackpotLamports) }} jackpot</template>.
              The house keeps {{ houseCut }}% and the league vault keeps {{ leagueCut }}% of ticket sales.
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
        <GameCardOptions :event="event" class="games-options" />
        <div v-for="(game, index) in event.games" :key="index" class="game-row">
          <div class="game-top">
            <div class="game-name-line">
              <div
                class="font-weight-medium team-names" :class="{ expanded: expandedNames.has(`r${index}`) }"
                :title="`${game.homeTeam} vs ${game.awayTeam}`" @click="toggleName(`r${index}`)"
              >
                <span class="game-no">{{ index + 1 }}.</span>{{ game.homeTeam }} vs {{ game.awayTeam }}
              </div>
              <MatchScore
                v-if="liveGame(index)?.linked"
                :game="liveGame(index)"
                :kickoff="game.kickoff"
                :placeholder="!game.result"
                :finished="game.result === '1' || game.result === 'X' || game.result === '2'"
              />
            </div>
            <span class="marks" :class="{ solo: marksFor(game).length === 1 }">
              <span v-for="mark in marksFor(game)" :key="mark" class="mark" :class="{ on: mark === outcomeAt(index) }">{{ mark }}</span>
            </span>
          </div>
          <GameFacts :game="game" :share="shareAt(index)" :started="started" />
          <div v-if="game.draw" class="draw-note text-caption text-medium-emphasis">
            Weights 1/X/2: {{ game.draw.weights.join(' / ') }} ·
            <a :href="explorerBlock(game.draw.slot)" target="_blank" rel="noopener">block {{ game.draw.slot }}</a>
          </div>
        </div>
        <v-card-text v-if="officialResults || showStanding">
          <div class="text-subtitle-2">All rows</div>
          <div v-if="!allRows.length" class="text-caption text-medium-emphasis">
            The totals appear once every match has a score.
          </div>
          <template v-else>
            <div class="text-caption text-medium-emphasis mb-1">
              <template v-if="officialResults">Every revealed row in this event.</template>
              <template v-else-if="projection?.maximum">If these scores stand. Other coupons are still hidden, so this is the most one row could win.</template>
              <template v-else>If these scores stand. Counted from revealed coupons. This is not the settled payout.</template>
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
          <v-alert v-if="pendingGroup" type="warning" variant="tonal" class="mb-4">
            {{ signedPieceCount }} of {{ pendingGroup.pieces.length }} coupons from your reduction are signed.
            The rest are saved. Resume signs only the ones that are not paid yet.
            <div class="mt-2 d-flex flex-wrap ga-2">
              <v-btn size="small" variant="text" :disabled="paying" @click="discardReduction">
                Drop the unsigned coupons
              </v-btn>
            </div>
          </v-alert>
          <div v-if="event.chain && !pendingGroup" class="reduce-panel mb-4">
            <div class="text-subtitle-2 mb-1">Reduce with colors</div>
            <p class="text-caption text-medium-emphasis mb-2">
              Choose a color, then tap signs to group them. A row is kept only when every color lands between its min and max.
              The reduction is one coupon with one signature, and you pay only for the rows it keeps.
            </p>
            <div class="d-flex flex-wrap ga-2 mb-2">
              <v-btn
                v-for="id in colorIds" :key="id" size="small" class="color-choice" :class="id"
                :variant="activeColor === id ? 'flat' : 'outlined'"
                @click="activeColor = activeColor === id ? '' : id"
              >
                {{ colorLabels[id] }}
                <span v-if="colorMarks[id].length" class="ml-1">{{ colorMarks[id].length }}</span>
              </v-btn>
            </div>
            <div v-for="id in usedColorIds" :key="id" class="color-limit">
              <span class="swatch" :class="id" />
              <span class="color-name">{{ colorLabels[id] }}</span>
              <label>Min <input type="number" min="0" :max="colorMarks[id].length" :value="colorLimits[id].min" :disabled="paying" @change="setLimit(id, 'min', $event)"></label>
              <label>Max <input type="number" min="0" :max="event.gameCount" :value="colorLimits[id].max" :disabled="paying" @change="setLimit(id, 'max', $event)"></label>
            </div>
          </div>
          <GameCardOptions :event="event" />
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
                <div
                  class="font-weight-medium team-names" :class="{ expanded: expandedNames.has(`p${g}`) }"
                  :title="`${game.homeTeam} vs ${game.awayTeam}`" @click="toggleName(`p${g}`)"
                >
                  <span class="game-no">{{ g + 1 }}.</span>{{ game.homeTeam }} vs {{ game.awayTeam }}
                </div>
                <MatchScore v-if="projection?.games?.[g]?.linked" :game="projection.games[g]" :kickoff="game.kickoff" />
              </div>
              <v-btn
                v-for="o in outcomes" :key="o.value" class="pick-btn" :title="o.label" :disabled="paying"
                :color="isSelected(g, o.value) && !signColors(g, o.value).length ? 'primary' : undefined"
                :variant="isSelected(g, o.value) ? 'flat' : 'outlined'"
                :class="signColors(g, o.value)"
                :style="signStyle(g, o.value)"
                @click="onPick(g, o.value)"
              >
                {{ o.value }}
              </v-btn>
            </div>
            <GameFacts :game="game" :share="shareAt(g)" :started="started" />
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
              <template v-if="reductionState?.ok && reductionState.rows !== combinationTotal">
                {{ combinationTotal.toLocaleString() }} reduced to {{ reductionState.rows.toLocaleString() }}
                ({{ removedPercent }}% removed) · 1 coupon ·
              </template>
              <template v-else-if="overRows">
                <span class="text-error">
                  {{ combinationTotal.toLocaleString() }} reduced to {{ overRows.toLocaleString() }}, over the
                  {{ maxCombinations.toLocaleString() }} limit
                </span>
                ·
              </template>
              <template v-else>
                {{ paidRows.toLocaleString() }} combination{{ paidRows === 1 ? '' : 's' }} ×
              </template>
              {{ event.league ? `${event.couponPriceLamports} RAD` : formatSol(event.couponPriceLamports) }} =
              <strong>{{ event.league ? `${totalLamports} RAD` : formatSol(totalLamports) }}</strong>
            </div>
            <div class="text-caption text-medium-emphasis">
              {{ statusText }}
            </div>
            <div class="text-caption text-medium-emphasis mt-1">
              <template v-if="event.league">This round is paid in RAD. You earn 1 RAD for every row on a SOL event. <span v-if="radBalance !== null">You have {{ radBalance }} RAD.</span></template>
              <template v-else>You pay the coupon price only, and this row earns 1 RAD toward the league event. A losing coupon keeps its stake in the pool. There is no account deposit and no monthly draw.</template>
            </div>
            <ProjectedPayout :system="systemRows(picks)" :colors="reductionState?.ok ? reductionState.colors : []" :projection="projection" />
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
            :disabled="!canSubmit || isTreasury" :loading="paying" @click="pay"
          >
            {{ payLabel }}
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
import GameCardOptions from '@/components/GameCardOptions.vue'
import GameFacts from '@/components/GameFacts.vue'
import MatchScore from '@/components/MatchScore.vue'
import ProjectedPayout from '@/components/ProjectedPayout.vue'
import VaultPanel from '@/components/VaultPanel.vue'
import { eventApi, reductionApi } from '@/services/api'
import { couponCapacity, fetchRad } from '@/services/program'
import { payAndEnter, placeOnChain, placeReduction } from '@/services/entry'
import { auth, loadConfig, login, refreshBalance } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { combinationCount, expandCombinations, systemRows } from '@/utils/coupons'
import { COLOR_IDS, colorReduction } from '@/utils/reduce'
import { eventStarted } from '@/services/gameCards'
import { showStandingPayout } from '@/services/projection'
import { explorerBlock, formatDate, formatSol, outcomes, statusColors } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const event = ref(null)
const projection = ref(null)
const loading = ref(false)
const refreshingScores = ref(false)
const fetchingOdds = ref(false)
const picks = ref([])
const paying = ref(false)
const step = ref('sign')
const signProgress = ref({ done: 0, total: 0 })
const colorIds = COLOR_IDS
const colorLabels = { purple: 'Purple', yellow: 'Yellow', red: 'Red', green: 'Green' }
const colorMarks = ref({ purple: [], yellow: [], red: [], green: [] })
const colorLimits = ref({
  purple: { min: 0, max: 0 }, yellow: { min: 0, max: 0 }, red: { min: 0, max: 0 }, green: { min: 0, max: 0 }
})
const activeColor = ref('')
const pendingGroup = ref(null)
const expandedNames = ref(new Set())

function toggleName (key) {
  const next = new Set(expandedNames.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedNames.value = next
}

const stepLabels = computed(() => event.value?.chain ? {
  sign: signProgress.value.total > 1
    ? `Approve coupon ${Math.min(signProgress.value.done + 1, signProgress.value.total)} of ${signProgress.value.total} in Phantom.`
    : 'Approve the entry in Phantom.',
  confirm: 'Waiting for the entry to be finalized on devnet...',
  schedule: 'Saving the time-locked picks. They are published when entries close.'
} : {
  sign: 'Approve the payment in Phantom (make sure Phantom is set to Devnet)...',
  confirm: 'Waiting for the payment to be finalized on devnet. This can take about a minute...',
  submit: 'Payment confirmed. Registering your coupons...'
})

const maxCombinations = computed(() => auth.config?.maxCombinations ?? auth.config?.maxCouponsPerEntry ?? 44000)
const houseCut = computed(() => auth.config?.houseCutPercent ?? 5)
const leagueCut = computed(() => auth.config?.leagueCutPercent ?? 5)
const radBalance = ref(null)
const isTreasury = computed(() => auth.config && auth.wallet === auth.config.treasuryWallet)
const settlement = computed(() => event.value?.settlement)
const combinationTotal = computed(() => combinationCount(picks.value))
const preview = computed(() => expandCombinations(picks.value))
const missingGames = computed(() => picks.value.filter(game => game.length === 0).length)
const usedColorIds = computed(() => colorIds.filter(id => colorMarks.value[id].length))
const reductionState = computed(() => {
  if (!usedColorIds.value.length || missingGames.value) return null
  const colors = usedColorIds.value.map(id => ({
    id,
    min: colorLimits.value[id].min,
    max: colorLimits.value[id].max,
    signs: colorMarks.value[id].map(key => {
      const [game, outcome] = key.split(':')
      return { game: Number(game), outcome }
    })
  }))
  try {
    return { ok: true, ...colorReduction(picks.value, colors, { maxCombinations: maxCombinations.value }) }
  } catch (error) {
    return { ok: false, error: error.message, rows: error.rows || 0 }
  }
})
const overRows = computed(() => reductionState.value?.ok === false ? reductionState.value.rows : 0)
const paidRows = computed(() => reductionState.value?.ok ? reductionState.value.rows : (overRows.value || combinationTotal.value))
const removedPercent = computed(() => {
  if (!reductionState.value?.ok || !combinationTotal.value) return 0
  return Math.round((1 - reductionState.value.rows / combinationTotal.value) * 100)
})
const signedPieceCount = computed(() => pendingGroup.value?.pieces.filter(piece => piece.status === 'signed').length || 0)
const tooMany = computed(() => !reductionState.value && combinationTotal.value > maxCombinations.value)
const chainCapacity = ref(null)
const limits = computed(() => {
  if (chainCapacity.value) return chainCapacity.value
  const current = event.value
  return {
    minRows: current?.minRows > 0 ? current.minRows : 1,
    maxCoupons: current?.maxCoupons > 0 ? current.maxCoupons : (1 << 20),
    count: current?.entryCount || 0
  }
})
const belowMin = computed(() => paidRows.value > 0 && paidRows.value < limits.value.minRows)
const eventFull = computed(() => limits.value.count >= limits.value.maxCoupons)
const totalLamports = computed(() => paidRows.value * (event.value?.couponPriceLamports ?? 0))
const complete = computed(() => missingGames.value === 0 && paidRows.value > 0 && !tooMany.value && !belowMin.value && !eventFull.value && reductionState.value?.ok !== false)
const canSubmit = computed(() => {
  if (eventFull.value) return false
  return pendingGroup.value ? signedPieceCount.value < pendingGroup.value.pieces.length : complete.value
})
const payLabel = computed(() => {
  if (pendingGroup.value) {
    const left = pendingGroup.value.pieces.length - signedPieceCount.value
    return `Resume ${left} coupon${left === 1 ? '' : 's'}`
  }
  return event.value?.league ? 'Enter with RAD' : 'Pay with Phantom'
})
const lowBalance = computed(() => !event.value?.league && auth.balance !== null && auth.balance * 1e9 < totalLamports.value + 10000)
const statusText = computed(() => {
  if (eventFull.value) return `This event is full at ${limits.value.maxCoupons.toLocaleString()} coupons.`
  if (pendingGroup.value) return 'The coupons already signed stay in the pool. Resume continues with the next one.'
  if (missingGames.value) return `${missingGames.value} game${missingGames.value === 1 ? '' : 's'} still need a pick`
  if (belowMin.value) return `This event requires at least ${limits.value.minRows.toLocaleString()} rows on a coupon. Add picks or loosen the colors.`
  if (reductionState.value?.ok === false) return reductionState.value.error
  if (tooMany.value) return `${combinationTotal.value.toLocaleString()} rows. A coupon can hold at most ${maxCombinations.value.toLocaleString()} kept rows. Use the colors to reduce it, or remove some picks.`
  if (reductionState.value?.ok && removedPercent.value === 0) return 'Min 0 and a max that covers every sign keeps every row. Lower the max or raise the min to remove rows.'
  if (activeColor.value) return `${colorLabels[activeColor.value]} is on. Tap a sign to add or remove it from that color.`
  return 'Plus a small Solana network fee, paid from your wallet.'
})

const salesLamports = computed(() => {
  const e = event.value
  if (!e) return 0
  return e.couponsSold * e.couponPriceLamports
})

// A vault event's carried-over jackpot is only on chain. The database copy is returned to the
// treasury when the vault is attached.
const jackpotLamports = computed(() => {
  if (event.value?.chain) return Number(chainCapacity.value?.rolloverIn ?? 0n)
  return event.value?.rolloverInLamports || 0
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

const started = computed(() => eventStarted(event.value))

function shareAt (index) {
  return event.value?.pickShare?.games?.[index] || null
}

async function fetchOdds () {
  fetchingOdds.value = true
  try {
    const saved = await eventApi.refreshOdds(event.value.id)
    event.value = saved.event
    if (saved.missing?.length) notify(`Odds saved. No line for ${saved.missing.join(', ')}.`, 'warning')
    else notify('Odds saved')
  } catch (error) {
    notifyError(error)
  } finally {
    fetchingOdds.value = false
  }
}

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

const showStanding = computed(() => showStandingPayout(projection.value))

const allRows = computed(() => {
  const source = projection.value?.maximum && !officialResults.value
    ? (projection.value.tiers || []).map(tier => ({
      correct: tier.correct,
      rows: tier.winners || 1,
      each: tier.perWinnerLamports || 0
    }))
    : (projection.value?.rows || [])
  const paying = new Set((event.value?.tiers || []).map(tier => tier.correct))
  if (!paying.size) return source
  return source.filter(row => paying.has(row.correct))
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
  colorMarks.value = { purple: [], yellow: [], red: [], green: [] }
  colorLimits.value = {
    purple: { min: 0, max: 0 }, yellow: { min: 0, max: 0 }, red: { min: 0, max: 0 }, green: { min: 0, max: 0 }
  }
  activeColor.value = ''
}

function isSelected (game, outcome) {
  return picks.value[game]?.includes(outcome)
}

function signKey (game, outcome) {
  return `${game}:${outcome}`
}

function signColors (game, outcome) {
  const key = signKey(game, outcome)
  return colorIds.filter(id => colorMarks.value[id].includes(key))
}

function signStyle (game, outcome) {
  const marks = signColors(game, outcome)
  if (!marks.length) return undefined
  const ink = { purple: '#6A1B9A', yellow: '#F9A825', red: '#C62828', green: '#2E7D32' }
  return { background: ink[marks[0]], color: marks[0] === 'yellow' ? '#1a1a1a' : '#fff', borderColor: 'transparent' }
}

function setLimit (id, field, input) {
  const raw = Number.parseInt(input.target.value, 10)
  const value = Number.isInteger(raw) && raw >= 0 ? raw : 0
  const limits = colorLimits.value[id]
  limits[field] = value
  if (limits.min > limits.max) {
    if (field === 'min') limits.max = limits.min
    else limits.min = limits.max
  }
}

function stripSign (game, outcome) {
  const key = signKey(game, outcome)
  for (const id of colorIds) {
    colorMarks.value[id] = colorMarks.value[id].filter(mark => mark !== key)
    const limits = colorLimits.value[id]
    if (limits.max > colorMarks.value[id].length) limits.max = colorMarks.value[id].length
    if (limits.min > limits.max) limits.min = limits.max
  }
}

function togglePick (game, outcome) {
  const selected = picks.value[game]
  const removing = selected.includes(outcome)
  picks.value[game] = removing
    ? selected.filter(value => value !== outcome)
    : [...selected, outcome]
  if (removing) stripSign(game, outcome)
}

function toggleColorSign (game, outcome) {
  const id = activeColor.value
  if (!id) return
  if (!isSelected(game, outcome)) togglePick(game, outcome)
  const key = signKey(game, outcome)
  const marks = colorMarks.value[id]
  if (marks.includes(key)) {
    colorMarks.value[id] = marks.filter(mark => mark !== key)
  } else {
    const tracking = colorLimits.value[id].max === marks.length
    colorMarks.value[id] = [...marks, key]
    if (tracking) colorLimits.value[id].max = colorMarks.value[id].length
  }
  const limits = colorLimits.value[id]
  if (limits.min > limits.max) limits.min = limits.max
}

function onPick (game, outcome) {
  if (activeColor.value) toggleColorSign(game, outcome)
  else togglePick(game, outcome)
}

function selectAll (outcome) {
  picks.value = picks.value.map(selected => selected.includes(outcome) ? selected : [...selected, outcome])
}

function clearPicks () {
  resetPicks()
}

async function loadRad () {
  radBalance.value = null
  if (!auth.wallet || !auth.config?.solanaRpcUrl) return
  try {
    const rad = await fetchRad(auth.config.solanaRpcUrl, auth.wallet)
    radBalance.value = rad.balance.toString()
  } catch {
    radBalance.value = '0'
  }
}

async function loadReduction () {
  pendingGroup.value = null
  if (!auth.wallet || !event.value?.chain || event.value.status !== 'open') return
  try {
    const { reductions } = await reductionApi.list(event.value.id)
    pendingGroup.value = (reductions || []).find(group => group.pieces.some(piece => piece.status !== 'signed')) || null
  } catch {
    pendingGroup.value = null
  }
}

async function discardReduction () {
  const group = pendingGroup.value
  if (!group || paying.value) return
  paying.value = true
  try {
    await reductionApi.remove(group.id)
    pendingGroup.value = null
    notify('The unsigned coupons were dropped. Coupons already signed stay in the pool.')
  } catch (error) {
    notifyError(error)
  } finally {
    paying.value = false
  }
}

async function load () {
  loading.value = true
  try {
    await loadConfig()
    event.value = await eventApi.get(route.params.id)
    await loadCapacity()
    if (event.value.status === 'open') resetPicks()
    loadRad()
    loadReduction()
  } catch (error) {
    notifyError(error)
    router.push('/')
  } finally {
    loading.value = false
  }
}

async function loadCapacity () {
  chainCapacity.value = null
  const address = event.value?.chain?.address
  if (!address) return
  try {
    chainCapacity.value = await couponCapacity(auth.config?.solanaRpcUrl, address)
  } catch {
    chainCapacity.value = null
  }
}

async function refreshEvent () {
  try {
    const fresh = await eventApi.get(route.params.id)
    const wasOpen = event.value?.status === 'open'
    event.value = fresh
    await loadCapacity()
    if (!wasOpen && fresh.status === 'open') resetPicks()
  } catch (error) {
    notifyError(error)
  }
}

async function pay () {
  paying.value = true
  signProgress.value = { done: 0, total: 0 }
  try {
    const reduced = !pendingGroup.value && reductionState.value?.ok && usedColorIds.value.length
      ? reductionState.value
      : null
    if (event.value.chain && pendingGroup.value) {
      const group = await placeReduction({
        config: auth.config,
        wallet: auth.wallet,
        event: event.value,
        group: pendingGroup.value,
        onStep: s => { step.value = s },
        onProgress: (done, total) => { signProgress.value = { done, total } }
      })
      notify(`All ${group.pieces.length} coupon${group.pieces.length === 1 ? '' : 's'} are signed. Your picks are published automatically when entries close.`)
      pendingGroup.value = null
    } else if (event.value.chain) {
      const placed = await placeOnChain({
        config: auth.config,
        wallet: auth.wallet,
        event: event.value,
        system: reduced ? reduced.system : systemRows(picks.value),
        combinations: reduced ? reduced.rows : combinationTotal.value,
        colors: reduced ? reduced.colors : [],
        onStep: s => { step.value = s }
      })
      notify(placed.armError
        ? `Coupon entered. The automatic reveal was not saved (${placed.armError}). Arm it from My Coupons.`
        : 'Coupon entered. Your picks are published automatically when entries close.')
    } else {
      await payAndEnter({
        config: auth.config,
        wallet: auth.wallet,
        event: event.value,
        system: systemRows(picks.value),
        combinations: combinationTotal.value,
        onStep: s => { step.value = s }
      })
      notify('Coupons placed. Good luck!')
    }
    refreshBalance()
    router.push('/coupons')
  } catch (error) {
    await loadReduction()
    if (error.placed) notify(`${error.message} ${error.placed} coupon${error.placed === 1 ? '' : 's'} already in the pool. Resume signs the rest.`, 'warning')
    else if (step.value === 'sign' || /No SOL was sent|failed on chain/.test(error.message)) notifyError(error)
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
  if (wallet && event.value) {
    loadProjection()
    loadRad()
    loadReduction()
  }
})

const projectionTimer = setInterval(() => {
  loadProjection()
  if (event.value && started.value && !event.value.pickShare) {
    eventApi.get(route.params.id).then(loaded => { event.value = loaded }).catch(() => {})
  }
}, 60000)
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
  --sign: 25px;
  --sign-gap: 8px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) repeat(3, var(--sign));
  column-gap: var(--sign-gap);
  align-items: center;
  row-gap: 1px;
  padding: 8px 0;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.game-top {
  display: grid;
  grid-template-columns: subgrid;
  grid-column: 1 / -1;
  align-items: center;
  min-width: 0;
}
.fill-row { border-bottom: 0; padding-bottom: 0; }
.fill-spacer { display: flex; justify-content: flex-end; min-width: 0; }
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
.team-names { cursor: pointer; }
.game-name-line .team-names.expanded {
  overflow: visible;
  white-space: normal;
  overflow-wrap: anywhere;
  text-overflow: clip;
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
.reduce-panel {
  padding: 12px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
}
.color-choice.purple { color: #6A1B9A; }
.color-choice.yellow { color: #F9A825; }
.color-choice.red { color: #C62828; }
.color-choice.green { color: #2E7D32; }
.color-limit {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  font-size: 13px;
}
.color-limit input {
  width: 64px;
  margin-left: 4px;
}
.swatch {
  width: 12px;
  height: 12px;
  border-radius: 3px;
}
.swatch.purple { background: #6A1B9A; }
.swatch.yellow { background: #F9A825; }
.swatch.red { background: #C62828; }
.swatch.green { background: #2E7D32; }
.marks {
  display: grid;
  grid-template-columns: subgrid;
  grid-column: 2 / -1;
}
.marks.solo { grid-template-columns: var(--sign); justify-self: end; }
.draw-note { grid-column: 1 / -1; }
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
