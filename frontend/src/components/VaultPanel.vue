<template>
  <v-card variant="outlined">
    <v-card-title class="d-flex align-center flex-wrap ga-2">
      <v-icon class="mr-1">mdi-safe</v-icon>
      On-chain vault
      <v-chip v-if="chain" size="small" variant="flat" :color="statusColor">{{ chain.status }}</v-chip>
      <v-chip v-if="paused" size="small" variant="flat" color="warning">entries paused</v-chip>
      <v-spacer />
      <v-btn icon="mdi-refresh" size="small" variant="text" :loading="loading" title="Reload from chain" @click="load" />
    </v-card-title>
    <v-card-subtitle class="text-wrap">
      <a :href="explorerAddress(address)" target="_blank" rel="noopener">{{ shortAddress(address) }}</a>
      · the program holds the entries and pays claims and refunds directly
    </v-card-subtitle>

    <v-card-text v-if="error">
      <v-alert type="warning" variant="tonal">{{ error }}</v-alert>
    </v-card-text>

    <v-card-text v-if="chain">
      <p>
        {{ chain.couponCount }} coupon(s) · sales {{ sol(chain.sales) }} · rollover in {{ sol(chain.rolloverIn) }}
      </p>
      <p class="text-caption text-medium-emphasis">
        Entries close {{ when(chain.deadline) }} · reveals and results from {{ when(chain.endTime) }} ·
        anyone can refund the event if it is not resolved by {{ when(chain.resolveTimeout) }}
      </p>
      <v-table density="compact" class="my-2">
        <thead>
          <tr><th>Correct</th><th>Share</th><th>{{ settled ? 'Rows' : 'Pot so far' }}</th><th v-if="settled">Each row wins</th></tr>
        </thead>
        <tbody>
          <tr v-for="tier in tierRows" :key="tier.correct">
            <td>{{ tier.correct }} of {{ chain.gameCount }}</td>
            <td>{{ tier.percent }}%</td>
            <td>{{ settled ? tier.rows : sol(tier.pot) }}</td>
            <td v-if="settled">{{ tier.rows ? sol(tier.perWinner) : 'rolls over' }}</td>
          </tr>
        </tbody>
      </v-table>
      <p v-if="settled">
        House {{ sol(chain.house) }}{{ chain.houseWithdrawn ? ' (withdrawn)' : '' }} · to winners {{ sol(chain.paid) }} ·
        rollover out {{ sol(chain.rolloverOut) }}{{ chain.rolloverForwarded ? ' (passed on)' : '' }}
      </p>
      <p v-if="chain.status === 'cancelled'">
        Every coupon can be refunded at its entry price. Rollover to pass on: {{ sol(chain.rolloverOut) }}{{ chain.rolloverForwarded ? ' (passed on)' : '' }}
      </p>
      <p v-if="chain.status === 'tallying'">
        Results {{ chain.results }} are locked. {{ chain.tallied }} of {{ chain.couponCount }} coupons counted.
      </p>

      <template v-if="chain.status === 'open' && chain.proposalKind !== 'none'">
        <div class="text-subtitle-2 mt-2">Pending {{ chain.proposalKind === 'cancel' ? 'cancellation' : `results ${chain.proposalResults}` }}</div>
        <div class="text-caption">
          Approved by {{ approvedBy.map(shortAddress).join(', ') || 'nobody' }} · {{ signers?.threshold }} of {{ signers?.members.length }} needed
        </div>
      </template>
    </v-card-text>

    <v-card-text v-if="chain && isSigner && chain.status === 'open'">
      <div class="text-subtitle-2 mb-1">Result signer</div>
      <p v-if="!resultsOpen" class="text-caption text-medium-emphasis mb-2">Results can be approved from {{ when(resultsFrom) }}.</p>
      <template v-else>
        <div v-for="(game, index) in event.games" :key="index" class="d-flex align-center flex-wrap ga-2">
          <span class="game-label">{{ index + 1 }}. {{ game.homeTeam }} vs {{ game.awayTeam }}</span>
          <v-btn-toggle v-model="draft[index]" density="compact" divided>
            <v-btn v-for="value in ['1', 'X', '2']" :key="value" :value="value" size="small">{{ value }}</v-btn>
            <v-btn value="C" size="small" color="warning" title="Void: every pick on this game counts as correct">Void</v-btn>
          </v-btn-toggle>
        </div>
        <p class="text-caption text-medium-emphasis mt-2">
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

    <v-card-actions v-if="chain" class="flex-wrap ga-2">
      <v-btn v-if="chain.status === 'tallying'" color="primary" variant="flat" :loading="busy === 'tally'" @click="tally">Count coupons and settle</v-btn>
      <v-btn v-if="canExpire" color="warning" variant="flat" :loading="busy === 'expire'" @click="expire">Refund the whole event (timeout)</v-btn>
      <v-btn v-if="canPassRollover" color="primary" variant="tonal" :loading="busy === 'rollover'" @click="passRollover">
        {{ destination ? 'Pass rollover to the next event' : 'Park rollover for the next event' }}
      </v-btn>
      <template v-if="isAdmin">
        <v-btn v-if="canWithdrawHouse" color="success" variant="tonal" :loading="busy === 'house'" @click="withdrawHouse">Withdraw house share {{ sol(chain.house) }}</v-btn>
        <v-btn variant="text" :loading="busy === 'pause'" @click="togglePause">{{ paused ? 'Resume entries' : 'Pause entries' }}</v-btn>
      </template>
    </v-card-actions>
    <v-alert v-if="progress" type="info" variant="tonal" class="ma-4 mt-0">{{ progress }}</v-alert>
  </v-card>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { eventApi } from '@/services/api'
import {
  TALLY_BATCH, approveCancelIx, approveResolutionIx, computeBudgetIx, eventCoupons, eventPda, expireEventIx, fetchEvent,
  fetchGatePaused, fetchSchedule, fetchSigners, forwardRolloverIx, parkRolloverIx, sendProgramTransaction, setPauseIx,
  tallyIx, withdrawHouseIx
} from '@/services/program'
import { waitForConfirmation } from '@/services/solana'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { explorerAddress, formatDate, formatSol, shortAddress } from '@/utils/format'

const props = defineProps({ event: { type: Object, required: true } })
const emit = defineEmits(['changed'])

const chain = ref(null)
const signers = ref(null)
const paused = ref(false)
const destination = ref(null)
const loading = ref(false)
const busy = ref(null)
const progress = ref('')
const error = ref('')
const draft = ref([])
const now = ref(Math.floor(Date.now() / 1000))

const address = computed(() => props.event.chain.address || eventPda(props.event.chain.admin, props.event.chain.nonce))
const admin = computed(() => props.event.chain.admin)
const rpcUrl = () => auth.config.solanaRpcUrl
const sol = lamports => formatSol(Number(lamports))
const when = seconds => formatDate(seconds * 1000)
const statusColor = computed(() => ({ open: 'success', tallying: 'primary', resolved: 'primary', cancelled: 'error' })[chain.value?.status] ?? 'default')
const settled = computed(() => chain.value?.status === 'resolved')
const isSigner = computed(() => signers.value?.members.includes(auth.wallet))
const isAdmin = computed(() => auth.wallet === admin.value)
const approvedBy = computed(() => (signers.value?.members ?? []).filter((_, index) => chain.value.approvals & (1 << index)))
const resultsFrom = computed(() => Math.max(chain.value.revealDeadline, chain.value.endTime))
const resultsOpen = computed(() => now.value >= resultsFrom.value && now.value < chain.value.resolveTimeout)
const draftComplete = computed(() => draft.value.length === chain.value?.gameCount && draft.value.every(Boolean))
const canExpire = computed(() => chain.value.status === 'open' && now.value >= chain.value.resolveTimeout)
const canPassRollover = computed(() => ['resolved', 'cancelled'].includes(chain.value.status) && chain.value.rolloverOut > 0n && !chain.value.rolloverForwarded)
const canWithdrawHouse = computed(() => settled.value && chain.value.house > 0n && !chain.value.houseWithdrawn)

const tierRows = computed(() => chain.value.tiers.map(tier => {
  const pot = chain.value.sales * BigInt(tier.percent) / 100n + (tier.correct === chain.value.gameCount ? chain.value.rolloverIn : 0n)
  return { ...tier, pot, rows: Number(chain.value.winners[tier.correct]), perWinner: chain.value.perWinner[tier.correct - 1] }
}))

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
    const [state, signerList, gate, schedule] = await Promise.all([
      fetchEvent(rpcUrl(), address.value), fetchSigners(rpcUrl(), address.value),
      fetchGatePaused(rpcUrl(), admin.value), fetchSchedule(rpcUrl(), admin.value)
    ])
    chain.value = state
    signers.value = signerList
    paused.value = gate
    const future = schedule.entries.filter(entry => entry.deadline > now.value && entry.event !== address.value)
    destination.value = future.sort((a, b) => a.deadline - b.deadline)[0]?.event ?? null
    if (draft.value.length !== state.gameCount) {
      draft.value = state.proposalKind === 'resolve' ? [...state.proposalResults] : Array(state.gameCount).fill(null)
    }
    if (state.status !== (props.event.chain?.status || 'open') && syncedFor !== `${props.event.id}:${state.status}`) {
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
  await send([approveCancelIx({ signer: auth.wallet, admin: admin.value, event: address.value })])
}, 'Cancellation approved')

// Tallying is permissionless and resumable: each batch continues from the on-chain counter.
const tally = () => run('tally', async () => {
  const coupons = await eventCoupons(rpcUrl(), address.value)
  let state = await fetchEvent(rpcUrl(), address.value)
  if (coupons.length < state.couponCount) throw new Error(`The RPC returned ${coupons.length} of ${state.couponCount} coupons; try again`)
  do {
    const batch = coupons.slice(state.tallied, state.tallied + TALLY_BATCH).map(coupon => coupon.address)
    progress.value = `Counting coupons ${state.tallied + 1}-${state.tallied + batch.length} of ${state.couponCount}...`
    await send([computeBudgetIx(1_400_000), tallyIx({ admin: admin.value, event: address.value, coupons: batch })])
    const before = state.tallied
    state = await fetchEvent(rpcUrl(), address.value)
    if (state.status === 'tallying' && state.tallied <= before) throw new Error('The count did not advance; reload and try again')
  } while (state.status === 'tallying')
}, 'Coupons counted and the event is settled')

const expire = () => run('expire', () => send([expireEventIx({ caller: auth.wallet, admin: admin.value, event: address.value })]), 'The event is refundable')

const passRollover = () => run('rollover', () => send([
  destination.value
    ? forwardRolloverIx({ admin: admin.value, source: address.value, destination: destination.value })
    : parkRolloverIx({ admin: admin.value, source: address.value })
]), 'Rollover passed on')

const withdrawHouse = () => run('house', () => send([withdrawHouseIx({ admin: auth.wallet, event: address.value })]), 'House share withdrawn')

const togglePause = () => run('pause', () => send([setPauseIx({ admin: auth.wallet, paused: !paused.value })]), paused.value ? 'Entries resumed' : 'Entries paused')

watch(() => props.event.id, load)
onMounted(load)
</script>

<style scoped>
.game-label { min-width: 220px; }
</style>
