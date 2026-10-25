<template>
  <v-container class="py-6" max-width="1000">
    <h1 class="text-h4 font-weight-light mb-2">My Coupons</h1>
    <v-checkbox
      v-model="hideResults" label="Hide each coupon until I open it"
      density="compact" hide-details class="mb-4"
    />

    <v-alert
      v-for="p in pending" :key="p.signature" type="warning" variant="tonal" class="mb-4"
      title="Payment not registered yet"
    >
      You paid {{ formatSol(p.lamports) }} for {{ (p.combinations || p.coupons?.length || 0).toLocaleString() }} combination(s) in {{ p.eventTitle }},
      but the coupons were not registered.
      <a :href="explorerTx(p.signature)" target="_blank" rel="noopener">View payment</a>
      <div v-if="expired(p)" class="mt-2">
        This payment is more than an hour old and can no longer become an entry. Request a verified refund below.
      </div>
      <template #append>
        <div class="d-flex flex-column ga-2">
          <v-btn v-if="!expired(p)" color="warning" variant="flat" :loading="retrying === p.signature" @click="retry(p)">Retry</v-btn>
          <v-btn variant="text" size="small" :loading="recovering === p.signature" @click="recover(p)">Request refund</v-btn>
          <v-btn variant="text" size="small" @click="dismiss(p)">Dismiss</v-btn>
        </div>
      </template>
    </v-alert>

    <v-card v-if="totalWinnings && !hideResults" color="success" variant="tonal" class="mb-4">
      <v-card-text class="text-h6">
        <v-icon class="mr-2">mdi-trophy</v-icon>
        Total winnings: {{ formatSol(totalWinnings) }}
        <div class="text-body-2 mt-1">Winnings are paid to your wallet by the admin from the devnet treasury.</div>
      </v-card-text>
    </v-card>

    <div class="d-flex flex-wrap align-center ga-2 mb-4">
      <span class="text-subtitle-1">Vault coupons</span>
      <v-spacer />
      <v-btn v-if="unrecovered" size="small" color="warning" variant="flat" :loading="recoveringChain" @click="recoverChain">
        Recover {{ unrecovered }} coupon(s) from chain
      </v-btn>
      <v-btn size="small" variant="text" prepend-icon="mdi-download" @click="exportBackup">Export backup</v-btn>
      <v-btn size="small" variant="text" prepend-icon="mdi-upload" @click="importInput?.click()">Import backup</v-btn>
      <input ref="importInput" type="file" accept="application/json" hidden @change="importBackup">
    </div>
    <v-alert v-if="chainError" type="warning" variant="tonal" class="mb-4">{{ chainError }}</v-alert>

    <v-card v-for="row in chainRows" :key="row.id" class="mb-4" variant="outlined">
      <v-card-title class="d-flex align-center flex-wrap ga-2" :class="{ 'open-row': hideResults }" @click="hideResults && toggle(row.id)">
        <router-link v-if="row.eventId" :to="`/events/${row.eventId}`" class="text-decoration-none" @click.stop>{{ row.eventTitle }}</router-link>
        <span v-else>{{ row.eventTitle }}</span>
        <v-spacer />
        <v-icon v-if="hideResults" size="small">{{ folded(row.id) ? 'mdi-chevron-down' : 'mdi-chevron-up' }}</v-icon>
        <v-chip size="small" variant="flat" :color="concealed(row.id, hasOutcome(row)) ? 'default' : row.color">{{ chipText(row) }}</v-chip>
      </v-card-title>
      <v-card-subtitle class="text-wrap">
        Coupon #{{ row.index + 1 }} · {{ row.combinations.toLocaleString() }} combination(s) · paid {{ formatSol(row.lamports) }}
        <template v-if="row.address && !row.slip"> · <a :href="explorerAddress(row.address)" target="_blank" rel="noopener">account</a></template>
      </v-card-subtitle>
      <v-card-text v-if="folded(row.id)" class="text-medium-emphasis open-row" @click="toggle(row.id)">
        {{ hasOutcome(row) ? 'Open this coupon to see if it won.' : 'Open this coupon to see how it is going.' }}
      </v-card-text>
      <v-card-text v-else>
        <CouponCompare v-if="row.system?.length" :picks="row.system" :results="shownResults(row.results, row.eventId)" :live="liveScores(row.results, row.eventId)" :games="row.games" />
        <CouponScore v-if="row.score" :best="row.score.best" :games="row.score.games" :lines="row.score.lines" />
        <ProjectedPayout v-if="row.system?.length && !row.results" :system="row.system" :projection="projections[row.eventId]" />
        <div v-if="row.detail" class="text-body-2 mb-2">{{ row.detail }}</div>
        <div class="d-flex flex-wrap ga-2">
          <v-btn v-if="row.canSchedule && !armed.includes(row.address)" color="primary" variant="flat" :loading="acting === row.id" @click="schedule(row)">Arm automatic reveal</v-btn>
          <v-btn v-if="row.action === 'reveal'" color="primary" variant="tonal" :loading="acting === row.id" @click="act(row, revealOnChain, 'Picks revealed')">Reveal now</v-btn>
          <v-btn v-if="row.action === 'claim'" color="success" variant="flat" :loading="acting === row.id" @click="act(row, claimOnChain, row.payout ? 'Winnings claimed.' : 'Coupon closed. The account rent was returned.')">
            {{ row.payout ? `Claim ${formatSol(row.payout)}` : 'Close coupon' }}
          </v-btn>
          <v-btn v-if="row.action === 'refund'" color="warning" variant="flat" :loading="acting === row.id" @click="act(row, refundOnChain, 'Entry refunded')">Refund {{ formatSol(row.lamports) }}</v-btn>
        </div>
        <p v-if="row.action === 'claim'" class="text-caption text-medium-emphasis mt-2 mb-0">
          <template v-if="row.payout">
            {{ formatSol(row.payout) }} is the prize. A slip has no account deposit. An older coupon account returns its rent to this wallet.
          </template>
          <template v-else>
            No prize, so the stake stays in the pool. Closing an older coupon returns its account rent. A slip has nothing further to claim.
          </template>
        </p>
      </v-card-text>
    </v-card>

    <v-progress-linear v-if="loading" indeterminate class="mb-4" />
    <v-card v-if="!loading && entries.length === 0" variant="outlined" class="pa-6 text-center">
      <p class="text-medium-emphasis mb-4">You have not placed any coupons yet.</p>
      <v-btn color="primary" to="/">Browse events</v-btn>
    </v-card>

    <v-card v-for="entry in entries" :key="entry.id" class="mb-4">
      <v-card-title
        class="d-flex align-center flex-wrap ga-2"
        :class="{ 'open-row': hideResults && !!entry.system?.length }"
        @click="hideResults && entry.system?.length && toggle(entry.id)"
      >
        <router-link :to="`/events/${entry.eventId}`" class="text-decoration-none" @click.stop>{{ entry.eventTitle || 'Event' }}</router-link>
        <v-chip v-if="entry.event" :color="statusColors[entry.event.status]" size="small" variant="flat">{{ entry.event.status }}</v-chip>
        <v-spacer />
        <v-icon v-if="hideResults && entry.system?.length" size="small">{{ folded(entry.id) ? 'mdi-chevron-down' : 'mdi-chevron-up' }}</v-icon>
        <v-chip v-if="entry.results && !hideResults" :color="entry.winningsLamports ? 'success' : 'default'" variant="flat">
          {{ entry.winningsLamports ? `Won ${formatSol(entry.winningsLamports)}` : 'No win' }}
        </v-chip>
      </v-card-title>
      <v-card-subtitle class="text-wrap">
        {{ (entry.combinations || entry.coupons?.length || 0).toLocaleString() }} combination(s) · paid {{ formatSol(entry.lamports) }} · {{ formatDate(entry.createdAt) }} ·
        <a :href="explorerTx(entry.signature)" target="_blank" rel="noopener">payment</a>
      </v-card-subtitle>
      <v-card-text>
        <template v-if="entry.system?.length">
          <div v-if="folded(entry.id)" class="text-medium-emphasis open-row" @click="toggle(entry.id)">
            {{ entrySettled(entry) ? 'Open this coupon to see if it won.' : 'Open this coupon to see how it is going.' }}
          </div>
          <template v-else>
            <CouponCompare :picks="entry.system" :results="shownResults(resultString(entry.event?.games), entry.eventId)" :live="liveScores(resultString(entry.event?.games), entry.eventId)" :games="entry.event?.games ?? []" />
            <CouponScore
              v-if="entry.scores?.length" class="mt-3"
              :best="entry.scores[0].correct" :games="entry.system.length"
              :lines="entry.scores.filter(item => item.winningsLamports).map(item => ({ correct: item.correct, rows: item.combinations, each: item.winningsLamports / item.combinations }))"
            />
            <div v-else-if="!shownResults(resultString(entry.event?.games), entry.eventId)" class="text-caption text-medium-emphasis mt-2">Waiting for results</div>
            <ProjectedPayout v-if="!entrySettled(entry)" :system="entry.system" :projection="projections[entry.eventId]" />
          </template>
        </template>
        <template v-else>
          <div v-for="(coupon, i) in entry.coupons" :key="i" class="py-2">
            <div class="d-flex align-center" :class="{ 'open-row': hideResults }" @click="hideResults && toggle(`${entry.id}:${i}`)">
              <span class="text-medium-emphasis">Coupon #{{ i + 1 }}</span>
              <v-spacer />
              <v-icon v-if="hideResults" size="small">{{ folded(`${entry.id}:${i}`) ? 'mdi-chevron-down' : 'mdi-chevron-up' }}</v-icon>
            </div>
            <div v-if="folded(`${entry.id}:${i}`)" class="text-caption text-medium-emphasis open-row" @click="toggle(`${entry.id}:${i}`)">
              {{ entry.results ? 'Open this coupon to see if it won.' : 'Open this coupon to see how it is going.' }}
            </div>
            <template v-else>
              <CouponCompare :picks="coupon.split('')" :results="shownResults(resultString(entry.event?.games), entry.eventId)" :live="liveScores(resultString(entry.event?.games), entry.eventId)" :games="entry.event?.games ?? []" />
              <ProjectedPayout v-if="!entry.results" :system="coupon.split('')" :projection="projections[entry.eventId]" />
              <CouponScore
                v-if="entry.results" class="mt-2"
                :best="entry.results[i].correct" :games="coupon.length"
                :lines="entry.results[i].winningsLamports ? [{ correct: entry.results[i].correct, rows: 1, each: entry.results[i].winningsLamports }] : []"
              />
              <div v-else-if="!shownResults(resultString(entry.event?.games), entry.eventId)" class="text-caption text-medium-emphasis mt-1">Waiting for results</div>
            </template>
          </div>
        </template>
      </v-card-text>
    </v-card>
  </v-container>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import CouponCompare from '@/components/CouponCompare.vue'
import CouponScore from '@/components/CouponScore.vue'
import ProjectedPayout from '@/components/ProjectedPayout.vue'
import { entryApi, eventApi } from '@/services/api'
import {
  COUPONS_REVEALED, PAYMENT_MAX_AGE_MS, armCouponReveal, backupMatches, claimOnChain, couponId, exportCouponBackup,
  importCouponBackup, loadArmed, loadChainCoupons, syncPublishedSlips,
  loadPending, recoverFromChain, refundOnChain, removePending, revealOnChain, submitPending
} from '@/services/entry'
import { couponPayout, couponScore, eventPda, fetchEvent, picksFromSystem, systemFromPicks, walletCoupons } from '@/services/program'
import { standingResults } from '@/services/projection'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { explorerAddress, explorerTx, formatDate, formatSol, statusColors } from '@/utils/format'

const entries = ref([])
const projections = ref({})
const pending = ref([])
const loading = ref(false)
const retrying = ref(null)
const recovering = ref(null)
const acting = ref(null)
const chainRows = ref([])
const chainError = ref('')
const unrecovered = ref(0)
const recoveringChain = ref(false)
const importInput = ref(null)
const armed = ref([])
const armTried = new Set()
const hideResults = ref(localStorage.getItem('sol.hideResults') !== '0')
const opened = ref({})
const outcomeLabels = new Set(['Won', 'No win', 'Not revealed', 'Claimed'])
watch(hideResults, value => localStorage.setItem('sol.hideResults', value ? '1' : '0'))

function toggle (id) {
  opened.value = { ...opened.value, [id]: !opened.value[id] }
}
function folded (id) {
  return hideResults.value && !opened.value[id]
}
function concealed (id, outcome) {
  return !!outcome && folded(id)
}
function hasOutcome (row) {
  return outcomeLabels.has(row.label)
}
function chipText (row) {
  if (concealed(row.id, hasOutcome(row))) return 'Settled'
  if (row.label !== 'Claimed') return row.label
  return row.payout ? `Claimed ${formatSol(row.payout)}` : 'Claimed · no win'
}
function scorePayout (score) {
  if (!score?.lines.length) return 0
  return Number(score.lines.reduce((sum, line) => {
    const each = typeof line.each === 'bigint' ? line.each : BigInt(Math.round(Number(line.each)))
    return sum + each * BigInt(line.rows)
  }, 0n))
}
function entrySettled (entry) {
  return !!(entry.results || entry.scores?.length)
}
let chainContext = { onChain: [], events: new Map() }

function describeSlip (saved, state, now) {
  if (saved.claimed) return { label: 'Claimed', color: 'success', detail: 'Paid from the vault.' }
  if (saved.refunded) return { label: 'Refunded', color: 'default', detail: 'The entry price was returned.' }
  if (!state) return { label: 'Entered', color: 'primary', detail: 'This coupon is in the archive.' }
  if (state.status === 'cancelled') return { label: 'Cancelled', color: 'warning', action: 'refund', detail: 'The event was cancelled. Refund the entry price to your wallet.' }
  if (state.status === 'resolved') {
    if (!saved.revealed) return { label: 'Not revealed', color: 'error', detail: 'The picks were not revealed in time, so the stake stayed in the prize pool.' }
    const score = couponScore({ revealed: true, picks: picksFromSystem(saved.system), gameCount: state.gameCount }, state)
    const payout = scorePayout(score)
    return {
      label: payout ? 'Won' : 'No win', color: payout ? 'success' : 'default',
      action: payout ? 'claim' : undefined, payout, score,
      detail: payout ? '' : 'No prize. The stake stayed in the pool.'
    }
  }
  if (state.status === 'tallying') return { label: 'Counting', color: 'primary', detail: 'Results are locked and coupons are being counted.' }
  if (now < state.deadline) return { label: 'Entered', color: 'primary', detail: `Your picks are published automatically when entries close (${formatDate(state.deadline * 1000)}).` }
  if (saved.revealed) return { label: 'Revealed', color: 'primary', detail: 'Waiting for both result signers.' }
  if (now >= state.revealDeadline) return { label: 'Not revealed', color: 'error', detail: 'The reveal window has closed; this coupon cannot win.' }
  return { label: 'Reveal now', color: 'warning', action: 'reveal', detail: `Reveal before ${formatDate(state.revealDeadline * 1000)} or the coupon cannot win.` }
}

function describe (onChain, saved, state, now) {
  const restorable = onChain && !onChain.revealed && backupMatches(auth.wallet, saved, onChain)
  if (!onChain) {
    if (saved?.slip) return describeSlip(saved, state, now)
    if (saved.claimed) return { label: 'Claimed', color: 'success', detail: 'Paid from the vault; the coupon account is closed.' }
    if (saved.refunded) return { label: 'Refunded', color: 'default', detail: 'The entry price was returned.' }
    if (Date.now() - saved.placedAt < 120000) return { label: 'Confirming', color: 'default', detail: 'Waiting for the entry to appear on chain.' }
    return { label: 'Closed', color: 'default', detail: 'This coupon is no longer on chain: it was claimed, refunded or never confirmed.' }
  }
  if (!state) return { label: 'Unknown event', color: 'default', detail: 'The on-chain event could not be read.' }
  if (state.status === 'cancelled') return { label: 'Cancelled', color: 'warning', action: 'refund', detail: 'The event was cancelled. Refund the entry price to your wallet.' }
  if (state.status === 'resolved') {
    const payout = couponPayout(onChain, state)
    return {
      label: payout ? 'Won' : (onChain.revealed ? 'No win' : 'Not revealed'),
      color: payout ? 'success' : 'default',
      action: 'claim',
      payout: Number(payout),
      score: onChain.revealed ? couponScore(onChain, state) : null,
      detail: onChain.revealed ? '' : 'The picks were not revealed in time, so the stake stayed in the prize pool.'
    }
  }
  if (state.status === 'tallying') return { label: 'Counting', color: 'primary', detail: 'Results are locked and coupons are being counted.' }
  if (state.proposalKind === 'cancel' && !restorable) {
    return { label: 'Cancel pending', color: 'warning', detail: 'A result signer approved cancelling this event. The refund opens once both result signers have approved.' }
  }
  if (now < state.deadline) return { label: 'Entered', color: 'primary', detail: `Your picks are published automatically when entries close (${formatDate(state.deadline * 1000)}), even if this page is closed.` }
  if (onChain.revealed) return { label: 'Revealed', color: 'primary', detail: 'Waiting for both result signers.' }
  if (now >= state.revealDeadline) return { label: 'Not revealed', color: 'error', detail: 'The reveal window has closed; this coupon cannot win.' }
  if (restorable) return { label: 'Reveal now', color: 'warning', action: 'reveal', detail: `Reveal before ${formatDate(state.revealDeadline * 1000)} or the coupon cannot win.` }
  return { label: 'Picks missing', color: 'error', detail: 'This browser does not have the picks. Recover them from chain or import a backup.' }
}

async function loadChain () {
  chainError.value = ''
  await loadConfig()
  const rpcUrl = auth.config.solanaRpcUrl
  await syncPublishedSlips(auth.wallet)
  const saved = loadChainCoupons(auth.wallet)
  let onChain = []
  const events = new Map()
  try {
    const list = await eventApi.list()
    for (const event of list) {
      if (event.chain) events.set(event.chain.address || eventPda(event.chain.admin, event.chain.nonce), event)
    }
    onChain = await walletCoupons(rpcUrl, auth.wallet)
  } catch (error) {
    chainError.value = `Could not read your vault coupons from chain: ${error.message}`
  }
  const addresses = new Set([...onChain.map(coupon => coupon.event), ...saved.map(coupon => eventPda(coupon.admin, coupon.nonce))])
  const states = new Map()
  const unreadable = []
  await Promise.all([...addresses].map(async address => {
    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        states.set(address, await fetchEvent(rpcUrl, address))
        return
      } catch (error) {
        if (attempt === 3) unreadable.push(error.message)
        else await new Promise(resolve => setTimeout(resolve, 1500 * attempt))
      }
    }
  }))
  if (unreadable.length && !chainError.value) chainError.value = `Could not read ${unreadable.length} vault event(s) from chain: ${unreadable[0]}. Reload to try again.`
  chainContext = { onChain, events }

  const savedById = new Map(saved.map(coupon => [couponId(coupon), coupon]))
  const localOnly = new Map(savedById)
  const now = Math.floor(Date.now() / 1000)
  const rows = []
  for (const coupon of onChain) {
    const local = savedById.get(coupon.address)
    localOnly.delete(coupon.address)
    const event = events.get(coupon.event)
    const state = states.get(coupon.event)
    const system = coupon.revealed ? systemFromPicks(coupon.picks, coupon.gameCount) : local?.system
    rows.push({
      id: coupon.address, address: coupon.address, index: coupon.index, combinations: coupon.combinations,
      lamports: Number(coupon.lamportsPaid), eventId: event?.id, eventTitle: event?.title || local?.eventTitle || 'Vault event',
      admin: event?.chain?.admin ?? state?.admin ?? local?.admin, nonce: event?.chain?.nonce ?? local?.nonce ?? state?.nonce,
      deadline: state?.deadline, revealDeadline: state?.revealDeadline,
      canSchedule: state?.status === 'open' && now < state.deadline && !coupon.revealed && !!local?.revealNonce && !!system,
      system, revealNonce: local?.revealNonce,
      results: state?.status === 'resolved' && coupon.revealed ? state.results : '',
      games: event?.games ?? [],
      ...describe(coupon, local, state, now)
    })
  }
  for (const local of localOnly.values()) {
    const address = local.admin != null ? eventPda(local.admin, local.nonce) : ''
    const event = events.get(address)
    const state = states.get(address)
    const settled = state?.status === 'resolved' && local.system?.length
    const described = describe(null, local, state, now)
    const score = settled ? couponScore({ revealed: true, picks: picksFromSystem(local.system), gameCount: state.gameCount }, state) : null
    const payout = scorePayout(score)
    rows.push({
      id: couponId(local), ...local,
      eventId: local.eventId || event?.id,
      eventTitle: local.eventTitle || event?.title || 'Vault event',
      games: event?.games ?? [],
      results: settled ? state.results : '',
      ...described,
      score,
      payout,
      detail: described.label === 'Claimed'
        ? (payout ? `Won ${formatSol(payout)} from the vault.` : 'No prize. The stake stayed in the pool.')
        : described.detail
    })
  }
  unrecovered.value = onChain.filter(coupon => {
    const state = states.get(coupon.event)
    const canReveal = state?.status === 'open' && now < state.revealDeadline
    return canReveal && !coupon.revealed && !backupMatches(auth.wallet, savedById.get(coupon.address), coupon)
  }).length
  chainRows.value = rows
  armQuiet(rows)
  loadProjections()
}

function armQuiet (rows) {
  for (const row of rows) {
    if (!row.canSchedule || armed.value.includes(row.address) || armTried.has(row.address)) continue
    armTried.add(row.address)
    armCouponReveal({
      wallet: auth.wallet, coupon: row,
      deadline: row.deadline, revealDeadline: row.revealDeadline
    }).then(() => {
      armed.value = loadArmed(auth.wallet)
    }).catch(() => {})
  }
}

async function schedule (row) {
  acting.value = row.id
  try {
    await loadConfig()
    await armCouponReveal({
      wallet: auth.wallet, coupon: row,
      deadline: row.deadline, revealDeadline: row.revealDeadline
    })
    notify('Automatic reveal saved. Your picks are published when entries close.')
  } catch (error) {
    notifyError(error)
  } finally {
    acting.value = null
    armed.value = loadArmed(auth.wallet)
    await loadChain()
  }
}

async function act (row, action, message) {
  acting.value = row.id
  try {
    await loadConfig()
    await action({ config: auth.config, wallet: auth.wallet, coupon: row })
    notify(message)
  } catch (error) {
    notifyError(error)
  } finally {
    acting.value = null
    await loadChain()
  }
}

async function recoverChain () {
  recoveringChain.value = true
  try {
    const restored = await recoverFromChain({ wallet: auth.wallet, ...chainContext })
    notify(`Recovered ${restored} coupon(s) from chain`)
  } catch (error) {
    notifyError(error)
  } finally {
    recoveringChain.value = false
    await loadChain()
  }
}

function exportBackup () {
  const blob = new Blob([exportCouponBackup(auth.wallet)], { type: 'application/json' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `sol-coupons-${auth.wallet.slice(0, 6)}.json`
  link.click()
  URL.revokeObjectURL(link.href)
}

async function importBackup (event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  try {
    notify(`Imported ${importCouponBackup(auth.wallet, await file.text())} coupon(s)`)
  } catch (error) {
    notifyError(error)
  }
  await loadChain()
}

const totalWinnings = computed(() => entries.value.reduce((sum, e) => sum + e.winningsLamports, 0))
const resultString = games => games?.length && games.every(game => game.result) ? games.map(game => game.result).join('') : ''
function shownResults (official, eventId) {
  if (official) return official
  return standingResults(projections.value[eventId])
}
function liveScores (official, eventId) {
  if (official) return []
  return projections.value[eventId]?.games || []
}
const expired = p => Date.now() - p.paidAt > PAYMENT_MAX_AGE_MS

async function load () {
  armed.value = loadArmed(auth.wallet)
  pending.value = loadPending(auth.wallet)
  loadChain().catch(notifyError)
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
    loadProjections()
  }
}

async function loadProjections () {
  const ids = new Set()
  for (const row of chainRows.value) if (row.eventId && !row.results) ids.add(row.eventId)
  for (const entry of entries.value) if (entry.eventId && !entry.results && !entry.scores?.length) ids.add(entry.eventId)
  if (!ids.size) {
    projections.value = {}
    return
  }
  const next = {}
  await Promise.all([...ids].map(async id => {
    try {
      next[id] = await eventApi.projection(id)
    } catch {
      // A missing score feed leaves the coupons usable.
    }
  }))
  projections.value = next
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

async function recover (p) {
  recovering.value = p.signature
  try {
    await entryApi.recover(p.eventId, p.system, p.signature)
    removePending(auth.wallet, p.signature)
    notify('Refund requested; the admin can now verify and return the payment')
  } catch (error) {
    notifyError(error)
  } finally {
    recovering.value = null
    pending.value = loadPending(auth.wallet)
  }
}

function dismiss (p) {
  removePending(auth.wallet, p.signature)
  pending.value = loadPending(auth.wallet)
}

const reloadChain = () => loadChain().catch(notifyError)
onMounted(() => {
  load()
  window.addEventListener(COUPONS_REVEALED, reloadChain)
})
onBeforeUnmount(() => window.removeEventListener(COUPONS_REVEALED, reloadChain))
const projectionTimer = setInterval(() => loadProjections(), 60000)
onBeforeUnmount(() => clearInterval(projectionTimer))
</script>

<style scoped>
.open-row { cursor: pointer; }
</style>

