<template>
  <component :is="embedded ? 'div' : VCard" v-bind="embedded ? {} : { variant: 'outlined' }">
    <v-card-title v-if="!brief" class="d-flex align-center flex-wrap ga-2">
      <v-icon class="mr-1">mdi-safe</v-icon>
      On-chain vault
      <v-chip v-if="chain" size="small" variant="flat" :color="statusColor">{{ chain.status }}</v-chip>
      <v-chip v-if="chain?.status === 'open' && chain.proposalKind !== 'none'" size="small" variant="tonal" color="warning">
        {{ chain.proposalKind }} pending · {{ approvedBy.length }} of {{ signers?.threshold }}
      </v-chip>
      <v-chip v-if="paused" size="small" variant="flat" color="warning">entries paused</v-chip>
      <v-spacer />
      <v-btn icon="mdi-refresh" size="small" variant="text" :loading="loading" title="Reload from chain" @click="load" />
    </v-card-title>
    <v-card-subtitle v-if="!brief" class="text-wrap">
      <a :href="explorerAddress(address)" target="_blank" rel="noopener">{{ shortAddress(address) }}</a>
      · the program holds the entries and pays claims and refunds directly
    </v-card-subtitle>

    <v-card-text v-if="error">
      <v-alert type="warning" variant="tonal">{{ error }}</v-alert>
    </v-card-text>

    <v-card-text v-if="chain && (!brief || chainStatusVisible)">
      <template v-if="!brief">
        <div class="sales-block mb-2">
          <div class="sales-label">Total pot</div>
          <div class="sales-amount">{{ sol(chain.sales + chain.rolloverIn) }}</div>
        </div>
        <p class="text-caption text-medium-emphasis mb-3">
          {{ sol(chain.sales) }} from {{ chain.couponCount }} coupon(s)<template v-if="chain.maxCoupons && chain.couponCount >= chain.maxCoupons"> (the event is full)</template><template v-if="chain.rolloverIn > 0n"> + {{ sol(chain.rolloverIn) }} jackpot</template>
        </p>
        <p class="text-caption text-medium-emphasis">
          Entries close {{ when(chain.deadline) }} · reveals and results from {{ when(chain.endTime) }} ·
          anyone can refund the event if it is not resolved by {{ when(chain.resolveTimeout) }}
        </p>
        <v-expansion-panels class="mt-2">
          <v-expansion-panel>
            <v-expansion-panel-title>{{ settled ? 'Winnings' : 'Pot so far' }}</v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-table density="compact">
                <thead>
                  <tr><th>Correct</th><th>Share</th><th>{{ settled ? 'Rows' : 'Pot so far' }}</th><th v-if="settled">Each row wins</th></tr>
                </thead>
                <tbody>
                  <tr v-for="tier in tierRows" :key="tier.correct">
                    <td>{{ tier.correct }} of {{ chain.gameCount }}</td>
                    <td>{{ tier.percent }}%</td>
                    <td>{{ settled ? tier.rows : sol(tier.pot) }}</td>
                    <td v-if="settled">{{ tier.perWinner ? sol(tier.perWinner) : 'rolls over' }}</td>
                  </tr>
                </tbody>
              </v-table>
              <p class="text-caption text-medium-emphasis mt-2 mb-0">
                A prize tier rolls over to the jackpot (all games correct) of the next {{ chain.league ? 'league' : (rolloverType?.short || 'Saturday') }} event when nobody hits it, or when each winning row would receive less than 0.01 SOL.
              </p>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </template>
      <v-alert v-if="auth.isAdmin && event.autoResolveBlock && resultsStillOpen" type="warning" variant="tonal" density="compact" class="mt-2">
        {{ event.autoResolveBlock }}
      </v-alert>
      <p v-if="settled">
        House {{ sol(chain.house) }}{{ chain.houseWithdrawn ? ' (withdrawn)' : '' }} · to winners {{ sol(chain.paid) }} ·
        rollover out {{ sol(chain.rolloverOut) }}{{ chain.rolloverForwarded ? ' (passed on)' : '' }}
      </p>
      <p v-if="settled && !brief" class="text-body-2 mt-2 mb-0">
        <router-link to="/coupons">My Coupons</router-link> shows each coupon you played, the games it hit, and what it won.
      </p>
      <p v-if="chain.status === 'cancelled'">
        Refund each coupon from <router-link to="/coupons">My Coupons</router-link>. Rollover to pass on: {{ sol(chain.rolloverOut) }}{{ chain.rolloverForwarded ? ' (passed on)' : '' }}
      </p>
      <div v-if="waitingForTotals" class="mt-2">
        <p>
          Results {{ chain.results }} are locked. The stored coupons are counted off chain.
          <template v-if="canSignTotals && signatureLocks">The other signer already approved this list, so this signature locks it. The reveal keeper co-signs and posts the bond. Any wallet can then challenge during the 6-hour window.</template>
          <template v-else-if="canSignTotals">Sign that list from this wallet. The server then locks it, and any wallet can challenge during the 6-hour window.</template>
          <template v-else-if="signedWaiting && otherMissing">You signed these totals in the browser, before the other signer. The bond stays where it is until that signer locks the list.</template>
          <template v-else-if="signedWaiting">You signed these totals. The server locks them next. Any wallet can then challenge during the 6-hour window.</template>
          <template v-else-if="cosignerWallet">This wallet is the result cosigner. It does not sign totals in the browser. The other result signer signs first, and this server locks the bond.</template>
          <template v-else-if="fallbackOpen">The 24-hour wait has passed. Count the coupons on chain to open claims.</template>
          <template v-else>A result signer signs the list from their wallet. Any wallet can challenge once it is locked.</template>
        </p>
        <p v-if="signedNames.length" class="mb-2">Signed by {{ signedNames.join(', ') }}.</p>
        <p v-if="previewError" class="text-caption">{{ previewError }}</p>
        <p v-if="previewRows.length" class="mb-2">
          <template v-for="(row, index) in previewRows" :key="row.score">
            <template v-if="index"> · </template>{{ row.count }} with {{ row.score }} correct
          </template>
        </p>
        <v-btn v-if="canSignTotals" color="primary" variant="flat" :loading="busy === 'sign-totals'" @click="signCountedTotals">
          {{ signatureLocks ? 'Sign and lock these totals' : 'Sign these totals' }}
        </v-btn>
      </div>
      <p v-else-if="chain.status === 'tallying'">
        Results {{ chain.results }} are locked. {{ chain.tallied }} of {{ chain.couponCount }} coupons counted.
      </p>
      <div v-if="showTotals" class="mt-2">
        <p v-if="totals?.rejected" class="mb-1">The signed totals were rejected. The recount is what this event pays.</p>
        <p v-else-if="chain.status === 'resolved' && chain.settlementPath === 2" class="mb-1">The signed totals were confirmed.</p>
        <p v-else-if="chain.status === 'window'" class="mb-1">{{ windowLeft }}</p>
        <p v-if="chain.status === 'recount'" class="mb-1">Recount {{ chain.tallied }} of {{ chain.couponCount }} coupons. Claims stay closed until it finishes.</p>
        <p v-if="signedRows.length" class="mb-1">
          <template v-for="(row, index) in signedRows" :key="row.score">
            <template v-if="index"> · </template>{{ row.count }} with {{ row.score }} correct
          </template>
        </p>
        <p v-if="chain.status === 'window'" class="text-caption text-medium-emphasis mb-2">
          The poster locked {{ sol(POSTER_BOND) }}. A challenge locks {{ sol(challengeBond) }}, 10% of the player pot within this event's limits.
        </p>
        <div class="d-flex flex-wrap ga-2">
          <v-btn v-if="auth.wallet && chain.status === 'window' && !challenged" color="warning" variant="flat" :loading="busy === 'challenge'" @click="challenge">
            Challenge these totals
          </v-btn>
          <v-btn v-if="auth.wallet && windowOver" color="primary" variant="flat" :loading="busy === 'finalize'" @click="finalize">
            Settle from the signed totals
          </v-btn>
        </div>
      </div>

      <template v-if="chain.status === 'open' && chain.proposalKind !== 'none'">
        <div class="text-subtitle-2 mt-2">Pending {{ chain.proposalKind === 'cancel' ? 'cancellation' : `results ${chain.proposalResults}` }}</div>
        <div class="text-caption">
          Approved by {{ approvedBy.map(shortAddress).join(', ') || 'nobody' }} · {{ signers?.threshold }} of {{ signers?.members.length }} needed.
          Nothing changes until every needed signer has approved.
        </div>
        <div v-if="isSigner && cosignerMissing" class="text-caption text-medium-emphasis mt-1">
          The other signer is the result cosigner {{ shortAddress(RESULT_COSIGNER) }}, which signs outside the browser:
          <code>node scripts/cosign-event.mjs {{ address }} {{ chain.proposalKind === 'cancel' ? 'cancel' : `resolve ${chain.proposalResults}` }}</code>
        </div>
      </template>
    </v-card-text>

    <v-card-text v-if="chain && isSigner && chain.status === 'open'">
      <div class="text-subtitle-2 mb-1">Result signer</div>
      <p v-if="!resultsOpen" class="text-caption text-medium-emphasis mb-2">Results can be approved from {{ when(resultsFrom) }}.</p>
      <template v-else>
        <p v-if="standingText" class="text-body-2 mb-2">
          Scores as they stand: <strong>{{ standingText }}</strong>
        </p>
        <p v-else class="text-caption text-medium-emphasis mb-2">
          The score feed has no result for every match yet. Refresh scores, then match each choice to the result shown beside the game.
        </p>
        <div v-for="(game, index) in event.games" :key="index" class="game-line mb-1">
          <div class="game-top">
            <span class="game-label"><span class="game-no">{{ index + 1 }}.</span>{{ game.homeTeam }} vs {{ game.awayTeam }}</span>
            <MatchScore :game="standingGame(index)" :kickoff="game.kickoff" />
            <strong v-if="standingGame(index)?.projected">{{ standingGame(index).projected }}</strong>
            <span v-if="standingGame(index)?.projected && !standingGame(index).final" class="text-caption text-medium-emphasis">still in play</span>
            <v-btn-toggle v-model="draft[index]" density="compact" divided>
            <v-btn v-for="value in ['1', 'X', '2']" :key="value" :value="value" size="small">{{ value }}</v-btn>
            <v-btn class="pick-void" value="C" size="small" color="warning" title="Void: every pick on this game counts as correct">Void</v-btn>
            </v-btn-toggle>
          </div>
          <span class="game-when">{{ kickoffStamp(game.kickoff) }}</span>
        </div>
        <div class="d-flex flex-wrap ga-2 mt-2">
          <v-btn size="small" variant="tonal" prepend-icon="mdi-check-all" :disabled="!standingReady" @click="useStanding">
            Use these results
          </v-btn>
          <v-btn v-if="auth.isAdmin" size="small" variant="text" prepend-icon="mdi-refresh" :loading="refreshingScores" @click="refreshStanding">
            Refresh scores
          </v-btn>
        </div>
        <p class="text-caption text-medium-emphasis mt-2">
          Use these results fills the choices from the scores above. A match still in play can change before you approve.
          A void game (postponed or abandoned) counts as correct for every coupon. Both result signers must approve
          exactly the same results, each from their own wallet.
        </p>
      </template>
      <div class="d-flex flex-wrap ga-2 mt-2">
        <v-btn v-if="resultsOpen" color="primary" variant="flat" :disabled="!draftComplete || alreadyApproved('resolve')" :loading="busy === 'resolve'" @click="approveResults">
          {{ alreadyApproved('resolve') ? 'You approved these results' : 'Approve results' }}
        </v-btn>
        <v-btn color="error" variant="tonal" :disabled="alreadyApproved('cancel')" :loading="busy === 'cancel'" @click="approveCancel">
          {{ alreadyApproved('cancel') ? 'You approved cancelling' : 'Approve cancelling the event' }}
        </v-btn>
      </div>
    </v-card-text>

    <v-card-text v-if="chain && isSigner && chain.status === 'window'">
      <p class="text-body-2 mb-2">The result signers can still cancel before the window ends. Every stake is then refunded, and the poster bond comes back.</p>
      <v-btn color="error" variant="tonal" :loading="busy === 'cancel'" @click="approveCancel">Approve cancelling the event</v-btn>
    </v-card-text>

    <v-card-actions v-if="chain" class="flex-wrap ga-2">
      <v-btn v-if="canCount" color="primary" variant="flat" :loading="busy === 'tally'" @click="tally">Count coupons and settle</v-btn>
      <v-btn v-if="canExpire" color="warning" variant="flat" :loading="busy === 'expire'" @click="expire">Refund the whole event (timeout)</v-btn>
      <v-btn v-if="canPassRollover" color="primary" variant="tonal" :loading="busy === 'rollover'" @click="passRollover">
        {{ rolloverLabel }}
      </v-btn>
      <v-btn v-if="canPassRollover && rolloverType && !destination" variant="text" @click="parkAndCreate">
        Park and create the next {{ rolloverType.short }} event
      </v-btn>
      <template v-if="isAdmin">
        <v-btn v-if="canWithdrawHouse" color="success" variant="tonal" :loading="busy === 'house'" @click="withdrawHouse">Withdraw house share {{ sol(chain.house) }}</v-btn>
        <v-btn variant="text" :loading="busy === 'pause'" @click="togglePause">{{ paused ? 'Resume entries' : 'Pause entries' }}</v-btn>
      </template>
    </v-card-actions>
    <v-alert v-if="progress" type="info" variant="tonal" class="ma-4 mt-0">{{ progress }}</v-alert>
  </component>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { VCard } from 'vuetify/components'
import MatchScore from '@/components/MatchScore.vue'
import { typeFromKind } from '@/services/eventTypes'
import { kickoffStamp } from '@/services/projection'
import { eventApi } from '@/services/api'
import { scoreNextSlip } from '@/services/entry'
import {
  CHALLENGE_SLOTS, POSTER_BOND, RESULT_COSIGNER, TALLY_BATCH, approveCancelIx, approveResolutionIx, challengeIx, computeBudgetIx, decodeTotals, disputeBond, eventCoupons, eventPda,
  compileMessage, expireEventIx, fetchEvent, fetchGatePaused, fetchKinds, fetchSchedule, fetchSigners, finalizeTotalsIx, forwardRolloverIx, openCountIx, parkRolloverIx, serializeUnsigned, signTotalsIx, signTotalsLockIx,
  sendProgramTransaction, setPauseIx, slipsPda, tallyIx, totalsPda, totalsSignatureLocks, withdrawHouseIx
} from '@/services/program'
import { getAccount, getSlot, latestBlockhash, signTransaction, toBase64, waitForConfirmation } from '@/services/solana'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { explorerAddress, formatDate, formatSol, shortAddress } from '@/utils/format'

const props = defineProps({
  event: { type: Object, required: true },
  embedded: { type: Boolean, default: false },
  brief: { type: Boolean, default: false }
})
const emit = defineEmits(['changed'])

const chain = ref(null)
const totals = ref(null)
const slot = ref(null)
const signers = ref(null)
const paused = ref(false)
const router = useRouter()
const destination = ref(null)
const rolloverType = computed(() => chain.value && !chain.value.league ? typeFromKind(chain.value.kind) : null)
const rolloverLabel = computed(() => {
  if (chain.value?.league) return 'Park the jackpot for the next league event'
  const name = rolloverType.value?.short || 'event'
  return destination.value ? `Pass rollover to the next ${name} event` : `Park rollover for the next ${name} event`
})
const loading = ref(false)
const busy = ref(null)
const progress = ref('')
const error = ref('')
const draft = ref([])
const projection = ref(null)
const preview = ref(null)
const previewError = ref('')
const refreshingScores = ref(false)
const now = ref(Math.floor(Date.now() / 1000))
let clock = 0

const address = computed(() => props.event.chain.address || eventPda(props.event.chain.admin, props.event.chain.nonce))
const admin = computed(() => props.event.chain.admin)
const rpcUrl = () => auth.config.solanaRpcUrl
const sol = lamports => formatSol(Number(lamports))
const when = seconds => formatDate(seconds * 1000)
const statusColor = computed(() => ({ open: 'success', tallying: 'primary', window: 'warning', recount: 'warning', resolved: 'primary', cancelled: 'error' })[chain.value?.status] ?? 'default')
const settled = computed(() => chain.value?.status === 'resolved')
const isSigner = computed(() => signers.value?.members.includes(auth.wallet))
const isAdmin = computed(() => auth.wallet === admin.value)
const approvedBy = computed(() => (signers.value?.members ?? []).filter((_, index) => chain.value.approvals & (1 << index)))
const resultsFrom = computed(() => Math.max(chain.value.revealDeadline, chain.value.endTime))
const resultsOpen = computed(() => now.value >= resultsFrom.value && now.value < chain.value.resolveTimeout)
const draftComplete = computed(() => draft.value.length === chain.value?.gameCount && draft.value.every(Boolean))
const standingText = computed(() => {
  const games = projection.value?.games
  if (!games?.length || games.length !== chain.value?.gameCount) return ''
  if (!games.every(game => ['1', 'X', '2'].includes(game.projected))) return ''
  return games.map(game => game.projected).join('')
})
const standingReady = computed(() => standingText.value.length === chain.value?.gameCount)
const canExpire = computed(() => chain.value.status === 'open' && now.value >= chain.value.resolveTimeout)
const canPassRollover = computed(() => ['resolved', 'cancelled'].includes(chain.value.status) && chain.value.rolloverOut > 0n && !chain.value.rolloverForwarded)
const cosignerMissing = computed(() => signers.value?.members.includes(RESULT_COSIGNER) && !approvedBy.value.includes(RESULT_COSIGNER))
const canWithdrawHouse = computed(() => settled.value && chain.value.house > 0n && !chain.value.houseWithdrawn)
const chainStatusVisible = computed(() => {
  const state = chain.value
  if (!state) return false
  return settled.value || state.status === 'cancelled' || state.status === 'tallying' || state.status === 'window' || state.status === 'recount' || state.proposalKind !== 'none' || !!(auth.isAdmin && props.event.autoResolveBlock)
})
const TOTALS_WAIT = 24 * 60 * 60
const resultsStillOpen = computed(() => !chain.value || chain.value.status === 'open')
const waitingForTotals = computed(() => chain.value?.status === 'tallying' && chain.value.settlementPath === 1 && !chain.value.countStarted)
const fallbackAt = computed(() => chain.value?.approvedAt ? chain.value.approvedAt + TOTALS_WAIT : 0)
const fallbackOpen = computed(() => waitingForTotals.value && fallbackAt.value > 0 && now.value >= fallbackAt.value)
const previewRows = computed(() => {
  const counts = preview.value?.totals
  if (!counts || !chain.value) return []
  return counts.map((count, score) => ({ score, count })).filter(row => row.score <= chain.value.gameCount && BigInt(row.count) > 0n)
})
const samePreview = computed(() => {
  const proposal = totals.value
  const counted = preview.value
  if (!proposal || !counted?.poster || proposal.locked || proposal.poster !== counted.poster) return false
  return proposal.totals.every((count, index) => count === BigInt(counted.totals[index]))
})
const alreadySignedPreview = computed(() => {
  if (!samePreview.value) return false
  const index = signers.value?.members.indexOf(auth.wallet) ?? -1
  return index >= 0 && (totals.value.approvals & (1 << index)) !== 0
})
const canSignTotals = computed(() => waitingForTotals.value && isSigner.value && auth.wallet !== RESULT_COSIGNER && !!preview.value?.poster && Array.isArray(preview.value?.totals) && !alreadySignedPreview.value)
const signedWaiting = computed(() => waitingForTotals.value && alreadySignedPreview.value)
const cosignerWallet = computed(() => waitingForTotals.value && auth.wallet === RESULT_COSIGNER && isSigner.value && !alreadySignedPreview.value)
const approvedSigners = computed(() => {
  if (!totals.value || totals.value.locked || !signers.value) return []
  if (preview.value && !samePreview.value) return []
  return signers.value.members.filter((_, index) => (totals.value.approvals & (1 << index)) !== 0)
})
const signedNames = computed(() => approvedSigners.value.map(shortAddress))
const otherMissing = computed(() => (signers.value?.members ?? []).some(member => member !== auth.wallet && !approvedSigners.value.includes(member)))
const signatureLocks = computed(() => totalsSignatureLocks({
  wallet: auth.wallet,
  members: signers.value?.members,
  threshold: signers.value?.threshold,
  approvals: totals.value?.approvals ?? 0,
  sameList: samePreview.value
}))
const canCount = computed(() => {
  const state = chain.value
  if (!state) return false
  if (state.status === 'recount' || fallbackOpen.value) return true
  return state.status === 'tallying' && (state.settlementPath !== 1 || state.countStarted === 1)
})
const showTotals = computed(() => chain.value && (chain.value.status === 'window' || chain.value.status === 'recount' || chain.value.settlementPath === 2))
const challenged = computed(() => totals.value && totals.value.challenger !== '11111111111111111111111111111111')
const challengeBond = computed(() => chain.value ? disputeBond(chain.value) : 0n)
const signedRows = computed(() => {
  const source = totals.value?.rejected ? totals.value.recount : totals.value?.totals
  if (!source || !chain.value) return []
  return source.map((count, score) => ({ score, count })).filter(row => row.score <= chain.value.gameCount && row.count > 0n)
    .map(row => ({ ...row, count: row.count.toString() }))
})
const windowOver = computed(() => {
  if (chain.value?.status !== 'window' || !totals.value || slot.value == null) return false
  return BigInt(slot.value) >= totals.value.windowSlot + CHALLENGE_SLOTS
})
const windowLeft = computed(() => {
  if (!totals.value) return 'Signed totals are locked. Claims stay closed during the challenge window.'
  if (slot.value == null) return 'The challenge window is open.'
  const end = totals.value.windowSlot + CHALLENGE_SLOTS
  if (BigInt(slot.value) >= end) return 'The window has ended. Anyone can settle.'
  const left = Number(end - BigInt(slot.value))
  const hours = Math.floor(left / 9000)
  const minutes = Math.round((left % 9000) / 150)
  return `${hours}h ${minutes}m left in the challenge window`
})

const tierRows = computed(() => chain.value.tiers.map(tier => {
  const pot = chain.value.sales * BigInt(tier.percent) / 100n + (tier.correct === chain.value.gameCount ? chain.value.rolloverIn : 0n)
  return { ...tier, pot, rows: Number(chain.value.winners[tier.correct]), perWinner: chain.value.perWinner[tier.correct - 1] }
}))

function standingGame (index) {
  return projection.value?.games?.[index] || null
}

function useStanding () {
  if (!standingReady.value) return
  draft.value = projection.value.games.map(game => game.projected)
}

function fillFinalScores () {
  const games = projection.value?.games
  if (!chain.value || !games || games.length !== chain.value.gameCount) return
  if (draft.value.some(Boolean)) return
  if (!games.every(game => game.final && ['1', 'X', '2'].includes(game.projected))) return
  draft.value = games.map(game => game.projected)
}

async function loadStanding () {
  try {
    projection.value = await eventApi.projection(props.event.id)
    fillFinalScores()
  } catch {
    projection.value = null
  }
}

async function refreshStanding () {
  refreshingScores.value = true
  try {
    projection.value = await eventApi.refreshScores(props.event.id)
    fillFinalScores()
  } catch (err) {
    notifyError(err)
  } finally {
    refreshingScores.value = false
  }
}

function alreadyApproved (kind) {
  if (!chain.value || chain.value.proposalKind !== kind || !approvedBy.value.includes(auth.wallet)) return false
  return kind === 'cancel' || chain.value.proposalResults === draft.value.join('')
}

let syncedFor = ''

async function load () {
  loading.value = true
  error.value = ''
  try {
    await loadConfig()
    now.value = Math.floor(Date.now() / 1000)
    const [state, signerList, gate, schedule, kinds] = await Promise.all([
      fetchEvent(rpcUrl(), address.value), fetchSigners(rpcUrl(), address.value),
      fetchGatePaused(rpcUrl(), admin.value), fetchSchedule(rpcUrl(), admin.value), fetchKinds(rpcUrl(), admin.value)
    ])
    chain.value = state
    totals.value = null
    slot.value = null
    preview.value = null
    previewError.value = ''
    if (state.status === 'window' || state.status === 'recount' || state.settlementPath === 2 || (state.status === 'tallying' && state.settlementPath === 1)) {
      const raw = await getAccount(rpcUrl(), totalsPda(address.value))
      totals.value = raw ? decodeTotals(raw) : null
    }
    if (state.status === 'window') slot.value = await getSlot(rpcUrl()).catch(() => null)
    signers.value = signerList
    paused.value = gate
    const future = schedule.entries
      .map((entry, index) => ({ ...entry, kind: kinds[index] ?? 0 }))
      .filter(entry => entry.deadline > now.value && entry.event !== address.value && entry.kind === state.kind)
    // A league jackpot only returns to the league vault. A SOL rollover only moves to the same event type.
    destination.value = state.league ? null : (future.sort((a, b) => a.deadline - b.deadline)[0]?.event ?? null)
    if (draft.value.length !== state.gameCount) {
      draft.value = state.proposalKind === 'resolve' ? [...state.proposalResults] : Array(state.gameCount).fill(null)
    }
    if (auth.wallet) await loadStanding()
    if (auth.wallet && state.status === 'tallying' && state.settlementPath === 1 && !state.countStarted && signerList?.members.includes(auth.wallet)) {
      try {
        preview.value = await eventApi.totals(props.event.id)
      } catch (err) {
        previewError.value = err.message
      }
    }
    if (auth.wallet && state.status !== (props.event.chain?.status || 'open') && syncedFor !== `${props.event.id}:${state.status}`) {
      syncedFor = `${props.event.id}:${state.status}`
      eventApi.syncChain(props.event.id).then(() => emit('changed')).catch(() => {})
    }
  } catch (err) {
    error.value = `Could not read the vault: ${err.message}`
  } finally {
    loading.value = false
  }
}

async function send (instructions) {
  const sent = await sendProgramTransaction(rpcUrl(), auth.wallet, instructions)
  await waitForConfirmation(rpcUrl(), sent.signature, sent.lastValidBlockHeight)
}

async function run (kind, task, message) {
  busy.value = kind
  try {
    await loadConfig()
    await task()
    notify(message)
  } catch (err) {
    notifyError(err)
  } finally {
    busy.value = null
    progress.value = ''
    await load()
    try {
      await eventApi.syncChain(props.event.id)
    } catch {
      // the chain stays authoritative; the listing catches up on the next sync
    }
    emit('changed')
  }
}

const signCountedTotals = () => run('sign-totals', async () => {
  const counted = preview.value
  if (!counted?.poster || !counted.totals) throw new Error('The counted totals are not loaded')
  if (signatureLocks.value) {
    const { value } = await latestBlockhash(rpcUrl())
    const compiled = compileMessage(auth.wallet, [signTotalsLockIx({
      signer: auth.wallet, poster: counted.poster, event: address.value, totals: counted.totals
    })], value.blockhash)
    const signed = await signTransaction(serializeUnsigned(compiled))
    let locked
    try {
      locked = await eventApi.lockTotals(props.event.id, toBase64(signed))
    } catch (err) {
      if (/404|not found/i.test(err.message)) throw new Error('The server cannot lock these totals yet. Update the backend, then sign again.')
      throw err
    }
    await waitForConfirmation(rpcUrl(), locked.signature, value.lastValidBlockHeight)
    return
  }
  await send([signTotalsIx({ signer: auth.wallet, poster: counted.poster, event: address.value, totals: counted.totals })])
}, signatureLocks.value ? 'Totals locked. The challenge window is open.' : 'Totals signed. The server locks them next.')

const approveResults = () => run('resolve', async () => {
  const results = draft.value.join('')
  if (chain.value.proposalKind === 'resolve' && chain.value.approvals && chain.value.proposalResults !== results &&
      !confirm(`The other signer approved ${chain.value.proposalResults}. Approving different results restarts the count. Continue?`)) {
    throw new Error('Approval cancelled')
  }
  await send([approveResolutionIx({ signer: auth.wallet, admin: admin.value, event: address.value, results })])
}, 'Results approved')

const approveCancel = () => run('cancel', async () => {
  if (!confirm('Approve cancelling this event? Once both signers approve, every coupon is refunded.')) throw new Error('Approval cancelled')
  const poster = chain.value.status === 'window' ? totals.value?.poster : undefined
  await send([approveCancelIx({ signer: auth.wallet, admin: admin.value, event: address.value, poster })])
}, 'Cancellation approved')

const challenge = () => run('challenge', async () => {
  const bond = sol(challengeBond.value)
  if (!confirm(`Lock ${bond} to challenge these totals? If the recount finds the same totals, this bond pays the recount and the rest joins the pool.`)) {
    throw new Error('Challenge cancelled')
  }
  await send([challengeIx({ challenger: auth.wallet, event: address.value })])
}, 'Challenge locked. The recount has started.')

const finalize = () => run('finalize', async () => {
  await send([finalizeTotalsIx({ caller: auth.wallet, admin: admin.value, event: address.value, poster: totals.value.poster })])
}, 'The event is settled from the signed totals')

const counting = status => status === 'tallying' || status === 'recount'

// Tallying is permissionless and resumable: each batch continues from the on-chain counter.
const tally = () => run('tally', async () => {
  let state = await fetchEvent(rpcUrl(), address.value)
  if (state.status === 'tallying' && state.settlementPath === 1 && !state.countStarted && (isAdmin.value || isSigner.value)) {
    await send([openCountIx({ signer: auth.wallet, event: address.value })])
    state = await fetchEvent(rpcUrl(), address.value)
  }
  // An event with a canopy has no slip book. Both are counted one coupon at a time.
  if (state.canopy || await getAccount(rpcUrl(), slipsPda(address.value))) {
    do {
      progress.value = `Counting coupon ${state.tallied + 1} of ${state.couponCount}...`
      await scoreNextSlip({ config: auth.config, wallet: auth.wallet, admin: admin.value, event: address.value })
      const before = state.tallied
      state = await fetchEvent(rpcUrl(), address.value)
      if (counting(state.status) && state.tallied <= before) throw new Error('The count did not advance; reload and try again')
    } while (counting(state.status))
    return
  }
  const coupons = await eventCoupons(rpcUrl(), address.value)
  if (coupons.length < state.couponCount) throw new Error(`The RPC returned ${coupons.length} of ${state.couponCount} coupons; try again`)
  do {
    const batch = coupons.slice(state.tallied, state.tallied + TALLY_BATCH).map(coupon => coupon.address)
    progress.value = `Counting coupons ${state.tallied + 1}-${state.tallied + batch.length} of ${state.couponCount}...`
    await send([computeBudgetIx(1_400_000), tallyIx({ admin: admin.value, event: address.value, coupons: batch })])
    const before = state.tallied
    state = await fetchEvent(rpcUrl(), address.value)
    if (counting(state.status) && state.tallied <= before) throw new Error('The count did not advance; reload and try again')
  } while (counting(state.status))
}, 'Coupons counted and the event is settled')

const expire = () => run('expire', () => send([expireEventIx({ caller: auth.wallet, admin: admin.value, event: address.value })]), 'The event is refundable')

function parkAndCreate () {
  const type = rolloverType.value
  if (!type) return
  router.push({ path: '/admin', query: { type: type.id, park: address.value } })
}

const passRollover = () => run('rollover', () => send([
  destination.value
    ? forwardRolloverIx({ admin: admin.value, source: address.value, destination: destination.value })
    : parkRolloverIx({ admin: admin.value, source: address.value })
]), 'Rollover passed on')

const withdrawHouse = () => run('house', () => send([withdrawHouseIx({ admin: auth.wallet, event: address.value })]), 'House share withdrawn')

const togglePause = () => run('pause', () => send([setPauseIx({ admin: auth.wallet, paused: !paused.value })]), paused.value ? 'Entries resumed' : 'Entries paused')

watch(() => [props.event.id, auth.wallet], load)
onMounted(() => {
  load()
  clock = setInterval(() => { now.value = Math.floor(Date.now() / 1000) }, 30000)
})
onUnmounted(() => clearInterval(clock))
</script>

<style scoped>
.game-line { display: flex; flex-direction: column; gap: 1px; width: 100%; }
.game-top { display: flex; align-items: center; flex-wrap: nowrap; gap: 8px; width: 100%; min-width: 0; }
.game-line :deep(.v-btn-toggle) { flex: none; }
.game-label { flex: 1 1 auto; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.game-no { margin-right: 3px; }
.game-when { font-size: 11px; line-height: 1.1; font-weight: 600; color: rgba(var(--v-theme-on-surface), 0.62); }
.game-line :deep(.v-btn) {
  width: 25px !important;
  min-width: 25px !important;
  height: 25px !important;
  padding: 0 !important;
  font-size: 12px;
}
.game-line :deep(.pick-void) { width: auto !important; padding-inline: 8px !important; }
@media (max-width: 959px) {
  .game-label { font-size: 10px; }
  .game-when { font-size: 9px; }
}
</style>
