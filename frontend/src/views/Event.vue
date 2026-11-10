<template>
  <v-container class="event-page py-6" max-width="480">
    <div class="topbar">
      <router-link to="/" class="back">
        <v-icon icon="mdi-arrow-left" size="18" />
        Events
      </router-link>
      <button type="button" class="info-btn" aria-label="How this event works" @click="infoOpen = true">
        <v-icon icon="mdi-information-outline" size="18" />
      </button>
      <v-dialog v-model="infoOpen" max-width="420">
        <v-card>
          <v-card-title class="d-flex align-center">
            How this works
            <v-spacer />
            <v-btn icon="mdi-close" variant="text" size="small" aria-label="Close" @click="infoOpen = false" />
          </v-card-title>
          <v-card-text class="text-body-2">
            <p v-if="event?.description" class="mb-3">{{ event.description }}</p>
            <p class="text-caption text-medium-emphasis mb-1">Picks</p>
            <p class="mb-3">1 is a home win, X a draw, 2 an away win. Pick one or more on every game. Every combination is its own paid row. 1 and X on one game, and a single pick on the rest, is 2 rows. Two games with two outcomes is 4 rows.</p>
            <template v-if="event?.chain">
              <p class="text-caption text-medium-emphasis mb-1">Reduce</p>
              <p class="mb-3">Choose a color, then tap signs to group them. A row is kept only when every color lands between its min and max. You sign once and pay only for the rows it keeps.</p>
            </template>
            <p class="text-caption text-medium-emphasis mb-1">Pool</p>
            <p v-if="event?.league" class="mb-3">This round is paid in RAD. You earn 1 RAD for every row on a SOL event.<span v-if="radBalance !== null"> You have {{ radBalance }} RAD.</span></p>
            <p v-else class="mb-3">You pay the row price only, plus a small Solana network fee. The row earns 1 RAD toward the league event. A losing coupon keeps its stake in the pool. The house keeps {{ houseCut }}% and the league vault keeps {{ leagueCut }}% of ticket sales.</p>
            <p class="text-caption text-medium-emphasis mb-1">Prize rows</p>
            <p v-for="tier in allocationTiers" :key="tier.correct" class="mb-1">
              {{ tier.correct }} correct receives {{ tier.percent }}% of ticket sales<span v-if="tier.correct === event?.gameCount">, plus any jackpot</span>.
            </p>
            <p class="mt-2 mb-3">A prize row with nobody on it, or one that would pay under 0.01 SOL, rolls into the jackpot of the next {{ typeName }} event.</p>
            <p v-if="settlement?.rolloverOutLamports" class="mb-3">
              {{ formatSol(settlement.rolloverOutLamports) }} rolled over to
              <router-link v-if="settlement.rolloverToEventId" :to="`/events/${settlement.rolloverToEventId}`">the next {{ typeName }} event</router-link>
              <span v-else>the next {{ typeName }} event that is created</span>.
            </p>
            <template v-if="event?.chain">
              <p class="text-caption text-medium-emphasis mb-1">Entries</p>
              <p class="mb-3">The on-chain vault holds the entries. Reveal your picks in My Coupons after entries close. A coupon that is not revealed before {{ formatDate(event.endTime) }} cannot win, and its stake stays in the pool.</p>
            </template>
            <p v-if="event && event.status !== 'open'" class="mb-3">A live score is what the match shows now. It is not the settled result. Standing payouts are counted from revealed coupons until the event is settled.</p>
            <p v-if="auth.isAdmin" class="mb-0">Scores also refresh 45 and 90 minutes after entries close. Fetch odds reads the Match Winner line once and saves it.</p>
          </v-card-text>
        </v-card>
      </v-dialog>
    </div>
    <v-progress-linear v-if="loading && !event" indeterminate class="mt-4" />

    <template v-if="event">
      <div class="hero">
        <div>
          <div class="balance">{{ event.league ? `${potLamports} RAD` : formatSol(potLamports) }}</div>
          <p class="balance-note">Total pot</p>
          <span v-if="jackpotLamports" class="event-tag is-jackpot">Jackpot {{ event.league ? `${jackpotLamports} RAD` : formatSol(jackpotLamports) }}</span>
        </div>
        <span class="type-mark" :class="{ jackpot: jackpotLamports }">
          <v-icon :icon="event.league ? 'mdi-ticket' : eventType.icon" :color="jackpotLamports ? '#F5B800' : eventType.color" size="22" />
          <span class="type-count">{{ event.gameCount }}</span>
        </span>
      </div>

      <div class="identity">
        <div class="event-title">
          {{ event.title }}
          <span class="event-tag" :class="statusTagClass">{{ statusTag }}</span>
        </div>
        <div class="event-meta">
          Closes {{ formatDate(event.startTime) }}
        </div>
      </div>

      <div v-if="auth.isAdmin && event.status !== 'completed'" class="chips">
        <button type="button" class="chip" :disabled="refreshingScores" title="Read the scores now" @click="refreshScores">Refresh scores</button>
        <button type="button" class="chip" :disabled="fetchingOdds" title="Save the Match Winner odds" @click="fetchOdds">Fetch odds</button>
      </div>

      <section v-if="event.status !== 'open'" class="block">
        <h2 class="section-title">Games</h2>
        <GameCardOptions :event="event" />
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
        <div v-if="allRows.length || (showStanding && !officialResults)" class="rows-block">
          <div class="section-title">All rows</div>
          <p v-if="!allRows.length" class="balance-note">The totals appear once every match has a score.</p>
          <template v-else>
            <p class="balance-note">{{ rowsCaption }}</p>
            <div v-for="row in allRows" :key="row.correct" class="result-row">
              <span>{{ row.correct }} correct</span>
              <span class="result-rows">{{ Number(row.rows).toLocaleString() }}</span>
              <span class="result-pay">{{ row.each ? formatSol(row.each) : 'rolls over' }}</span>
            </div>
          </template>
        </div>
        <v-btn v-if="event.status === 'completed' && auth.wallet" color="primary" size="large" class="play coupons-link" prepend-icon="mdi-ticket" to="/coupons">
          See my coupons
        </v-btn>
      </section>

      <section v-if="event.status === 'open'" class="block">
        <div class="picks-head">
          <h2 class="section-title">Your picks</h2>
          <button v-if="event.chain && !pendingGroup" type="button" class="chip" :class="{ on: showReduce }" @click="showReduce = !showReduce">
            Reduce
          </button>
        </div>
        <v-alert v-if="isTreasury" type="warning" variant="tonal" class="mb-3">
          This wallet is the treasury wallet and cannot pay itself. Sign in with another wallet to play.
        </v-alert>
        <v-alert v-if="pendingGroup" type="warning" variant="tonal" class="mb-3">
          {{ signedPieceCount }} of {{ pendingGroup.pieces.length }} coupons from your reduction are signed. Resume signs the rest.
          <div class="mt-2">
            <v-btn size="small" variant="text" :disabled="paying" @click="discardReduction">Drop the unsigned coupons</v-btn>
          </div>
        </v-alert>
        <div v-if="showReduce && event.chain && !pendingGroup" class="reduce-panel">
          <div class="d-flex flex-wrap ga-2">
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
            <span class="stepper">
              <span class="step-label">Min</span>
              <button type="button" class="step-btn" :disabled="paying || colorLimits[id].min <= 0" :aria-label="`Lower ${colorLabels[id]} minimum`" @click="stepLimit(id, 'min', -1)">
                <v-icon icon="mdi-chevron-down" size="16" />
              </button>
              <span class="step-value">{{ colorLimits[id].min }}</span>
              <button type="button" class="step-btn" :disabled="paying || colorLimits[id].min >= colorMarks[id].length" :aria-label="`Raise ${colorLabels[id]} minimum`" @click="stepLimit(id, 'min', 1)">
                <v-icon icon="mdi-chevron-up" size="16" />
              </button>
            </span>
            <span class="stepper">
              <span class="step-label">Max</span>
              <button type="button" class="step-btn" :disabled="paying || colorLimits[id].max <= 0" :aria-label="`Lower ${colorLabels[id]} maximum`" @click="stepLimit(id, 'max', -1)">
                <v-icon icon="mdi-chevron-down" size="16" />
              </button>
              <span class="step-value">{{ colorLimits[id].max }}</span>
              <button type="button" class="step-btn" :disabled="paying || colorLimits[id].max >= colorMarks[id].length" :aria-label="`Raise ${colorLabels[id]} maximum`" @click="stepLimit(id, 'max', 1)">
                <v-icon icon="mdi-chevron-up" size="16" />
              </button>
            </span>
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
        <div v-if="preview.length" class="combos">
          <div class="section-title">Combinations</div>
          <div class="d-flex flex-column ga-2">
            <CouponPicks v-for="coupon in preview" :key="coupon" :coupon="coupon" :games="event.games" />
          </div>
        </div>
        <ProjectedPayout :system="systemRows(picks)" :colors="reductionState?.ok ? reductionState.colors : []" :projection="projection" />
        <v-alert v-if="paying" type="info" variant="tonal" density="compact" class="mt-3">{{ stepLabels[step] }}</v-alert>
        <div class="pay-dock">
          <div class="pay-hero">
            <div class="pay-copy">
              <p class="pay-total">{{ event.league ? `${totalLamports} RAD` : formatSol(totalLamports) }}</p>
              <p class="pay-line">{{ paidRows.toLocaleString() }} × {{ priceLabel }}</p>
              <p class="pay-line" :class="{ 'is-short': lowBalance }">Balance = {{ walletBalance }}</p>
            </div>
            <v-btn
              v-if="!auth.wallet"
              color="primary" variant="flat" size="large" class="play"
              :loading="auth.busy" @click="onLogin"
            >
              Connect
            </v-btn>
            <v-btn
              v-else
              color="primary" variant="flat" size="large" class="play"
              :disabled="!canSubmit || isTreasury" :loading="paying" @click="pay"
            >
              {{ payLabel }}
            </v-btn>
          </div>
        </div>
      </section>

      <VaultPanel v-if="event.chain" brief embedded class="vault-slot" :event="event" @changed="refreshEvent" />
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
import { eventTypeOf } from '@/services/eventTypes'
import { couponCapacity, fetchRad } from '@/services/program'
import { payAndEnter, placeOnChain, placeReduction } from '@/services/entry'
import { auth, loadConfig, login, refreshBalance } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { combinationCount, expandCombinations, systemRows } from '@/utils/coupons'
import { COLOR_IDS, colorPaint, colorReduction } from '@/utils/reduce'
import { eventStarted } from '@/services/gameCards'
import { showStandingPayout } from '@/services/projection'
import { explorerBlock, formatDate, formatSol, outcomes } from '@/utils/format'

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
const infoOpen = ref(false)
const showReduce = ref(false)

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
const eventType = computed(() => eventTypeOf(event.value))
const typeName = computed(() => event.value?.league ? 'league' : eventType.value.short)
const priceLabel = computed(() => event.value?.league ? `${event.value.couponPriceLamports} RAD` : formatSol(event.value?.couponPriceLamports || 0))
const allocationTiers = computed(() => {
  const tiers = settlement.value?.tiers || event.value?.tiers || []
  return [...tiers].sort((a, b) => b.correct - a.correct)
})
const statusTag = computed(() => {
  if (event.value?.status === 'closed') return 'Live'
  if (event.value?.status === 'completed') return ''
  return 'Open'
})
const statusTagClass = computed(() => event.value?.status === 'closed' ? 'is-live' : '')
const rowsCaption = computed(() => {
  if (officialResults.value) return 'Every revealed row.'
  if (projection.value?.maximum) return 'Most one row could win if these scores stand.'
  return 'If these scores stand. Not the settled payout.'
})
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
  return event.value?.league ? 'Enter' : 'Pay'
})
const lowBalance = computed(() => !event.value?.league && auth.balance !== null && auth.balance * 1e9 < totalLamports.value + 10000)
const walletBalance = computed(() => {
  if (event.value?.league) return radBalance.value === null ? '—' : `${radBalance.value} RAD`
  if (!auth.wallet || auth.balance === null) return '—'
  return formatSol(Math.round(auth.balance * 1e9))
})

const salesLamports = computed(() => {
  const e = event.value
  if (!e) return 0
  if (e.chain && chainCapacity.value) return Number(chainCapacity.value.sales || 0)
  return e.couponsSold * e.couponPriceLamports
})
const potLamports = computed(() => salesLamports.value + jackpotLamports.value)

// A vault event's carried-over jackpot is only on chain. The database copy is returned to the
// treasury when the vault is attached.
const jackpotLamports = computed(() => {
  if (event.value?.chain) return Number(chainCapacity.value?.rolloverIn ?? 0n)
  return event.value?.rolloverInLamports || 0
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
  return colorPaint(signColors(game, outcome))
}

// Min and max move one step at a time, between 0 and the number of signs in the color.
function stepLimit (id, field, delta) {
  const limits = colorLimits.value[id]
  const cap = colorMarks.value[id].length
  const next = limits[field] + delta
  if (next < 0 || next > cap) return
  limits[field] = next
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
  if (limits.max > colorMarks.value[id].length) limits.max = colorMarks.value[id].length
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
.event-page { padding-bottom: 24px; }
.topbar { display: flex; align-items: center; justify-content: space-between; }
.back { display: inline-flex; align-items: center; gap: 4px; color: rgba(var(--v-theme-on-surface), 0.62); text-decoration: none; font-size: var(--fs-small); font-weight: 600; }
.info-btn { width: 34px; height: 34px; border: 0; border-radius: 10px; background: rgb(var(--v-theme-surface)); color: rgba(var(--v-theme-on-surface), 0.62); display: inline-flex; align-items: center; justify-content: center; cursor: pointer; }
.hero { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 8px; }
.balance { font-size: var(--fs-display); line-height: var(--lh-display); font-weight: 700; letter-spacing: -0.02em; }
.balance-note { margin: 2px 0 0; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.type-mark { width: 44px; height: 44px; border-radius: 14px; background: rgb(var(--v-theme-surface-variant)); display: flex; align-items: center; justify-content: center; position: relative; flex: none; }
.type-mark.jackpot { background: rgba(245, 184, 0, 0.16); }
.type-count { position: absolute; right: -4px; bottom: -4px; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 999px; background: rgb(var(--v-theme-background)); color: rgb(var(--v-theme-on-surface)); font-size: 10px; font-weight: 800; display: flex; align-items: center; justify-content: center; }
.identity { margin-top: 14px; }
.event-title { font-size: var(--fs-body); font-weight: 600; }
.event-meta { margin-top: 2px; font-size: var(--fs-small); color: rgba(var(--v-theme-on-surface), 0.62); }
.event-tag { display: inline-block; margin-top: 4px; font-size: var(--fs-small); font-weight: 600; color: rgba(var(--v-theme-warning), 0.62); }
.event-tag.is-jackpot { color: #F5B800; }
.event-tag.is-live { color: rgb(var(--v-theme-success)); }
.chips { display: flex; gap: 8px; margin-top: 12px; }
.chip { height: 34px; padding: 0 12px; border: 0; border-radius: 10px; background: rgb(var(--v-theme-surface)); color: rgba(var(--v-theme-on-surface), 0.62); font: inherit; font-size: var(--fs-body); font-weight: 600; cursor: pointer; }
.chip.on { background: rgb(var(--v-theme-surface-variant)); color: rgb(var(--v-theme-on-surface)); }
.chip:disabled { opacity: 0.5; }
.block { margin-top: 22px; }
.block :deep(.card-options) { align-items: flex-start; }
.block :deep(.option-grid) { justify-content: flex-start; }
.picks-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.section-title { font-size: var(--fs-heading); line-height: var(--lh-heading); font-weight: 600; margin: 0 0 8px; }
.rows-block { margin-top: 16px; }
.result-row { display: flex; align-items: center; gap: 12px; height: 40px; font-size: var(--fs-small); }
.result-rows { margin-left: auto; color: rgba(var(--v-theme-on-surface), 0.62); }
.result-pay { font-weight: 700; min-width: 72px; text-align: right; }
.coupons-link { margin-top: 16px; }
.combos { margin-top: 16px; }
.pay-dock { margin-top: 16px; }
.pay-hero { display: flex; align-items: center; justify-content: space-between; gap: 16px; height: 52px; position: sticky; bottom: 0; z-index: 2; background: rgb(var(--v-theme-background)); border-top: 1px solid var(--stroke); }
.pay-copy { min-width: 0; flex: 1; display: flex; flex-direction: column; justify-content: center; gap: 1px; }
.pay-total { margin: 0; height: 20px; font-size: var(--fs-heading); line-height: 20px; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pay-line { margin: 0; height: 14px; font-size: var(--fs-label); line-height: 14px; font-weight: 600; color: rgba(var(--v-theme-on-surface), 0.72); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pay-line.is-short { color: rgb(var(--v-theme-error)); }
.play { min-width: 128px; border-radius: 14px; }
.vault-slot { margin-top: 20px; }
.game-row {
  --sign: 36px;
  --sign-gap: 8px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) repeat(3, var(--sign));
  column-gap: var(--sign-gap);
  align-items: center;
  row-gap: 1px;
  padding: 8px 0;
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
  flex: 0 0 36px;
  width: 36px !important;
  min-width: 36px !important;
  height: 32px !important;
  min-height: 32px !important;
  padding: 0 !important;
  font-size: 12px;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease, transform 120ms ease;
}
.pick-btn:active {
  transform: scale(0.98);
}
.pay-bar {
  position: sticky;
  bottom: 0;
  z-index: 2;
  background: rgb(var(--v-theme-surface));
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
  gap: 12px;
  margin-top: 8px;
  font-size: 13px;
  user-select: none;
}
.color-name { width: 48px; flex: none; }
.stepper { display: inline-flex; align-items: center; gap: 4px; flex: none; }
.step-label { margin-right: 2px; color: rgba(var(--v-theme-on-surface), 0.62); }
.step-value { min-width: 14px; text-align: center; font-weight: 700; }
.step-btn {
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: rgb(var(--v-theme-surface-variant));
  color: rgb(var(--v-theme-on-surface));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.step-btn:disabled { opacity: 0.35; cursor: default; }
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
  width: 36px;
  min-width: 36px;
  height: 32px;
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
  .hero { margin-top: 4px; }
  .game-row { flex-wrap: nowrap; width: 100%; }
  .game-name-line .font-weight-medium { font-size: 14px; }
  .game-when { font-size: 9px; }
  .event-page:has(.pay-dock) { padding-bottom: 120px !important; }
  .pay-dock {
    position: fixed;
    left: 16px;
    right: 16px;
    bottom: calc(16px + var(--nav-height) + 10px + var(--inset-bottom));
    z-index: 6;
    box-sizing: border-box;
    height: 72px;
    margin: 0;
    padding: 10px 12px;
    overflow: hidden;
    border-radius: var(--radius-lg);
    background: rgb(var(--v-theme-surface));
  }
  .pay-hero { position: static; margin: 0; border: 0; background: transparent; }
  .play { align-self: center; }
}
</style>
