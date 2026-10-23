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

    <v-card v-if="totalWinnings" color="success" variant="tonal" class="mb-4">
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
      <v-card-title class="d-flex align-center flex-wrap ga-2">
        <router-link v-if="row.eventId" :to="`/events/${row.eventId}`" class="text-decoration-none">{{ row.eventTitle }}</router-link>
        <span v-else>{{ row.eventTitle }}</span>
        <v-spacer />
        <v-chip size="small" variant="flat" :color="row.color">{{ row.label }}</v-chip>
      </v-card-title>
      <v-card-subtitle class="text-wrap">
        Coupon #{{ row.index + 1 }} · {{ row.combinations.toLocaleString() }} combination(s) · paid {{ formatSol(row.lamports) }}
        <template v-if="row.address"> · <a :href="explorerAddress(row.address)" target="_blank" rel="noopener">account</a></template>
      </v-card-subtitle>
      <v-card-text>
        <div v-if="row.system" class="mb-2">
          <span v-for="(picks, game) in row.system" :key="game" class="mr-2">{{ game + 1 }}:{{ picks }}</span>
        </div>
        <div class="text-body-2 mb-2">{{ row.detail }}</div>
        <div class="d-flex flex-wrap ga-2">
          <v-btn v-if="row.action === 'reveal'" color="primary" variant="flat" :loading="acting === row.id" @click="act(row, revealOnChain, 'Picks revealed')">Reveal picks</v-btn>
          <v-btn v-if="row.action === 'claim'" color="success" variant="flat" :loading="acting === row.id" @click="act(row, claimOnChain, row.payout ? 'Winnings claimed' : 'Coupon closed and rent returned')">
            {{ row.payout ? `Claim ${formatSol(row.payout)}` : 'Close coupon (returns rent)' }}
          </v-btn>
          <v-btn v-if="row.action === 'refund'" color="warning" variant="flat" :loading="acting === row.id" @click="act(row, refundOnChain, 'Entry refunded')">Refund {{ formatSol(row.lamports) }}</v-btn>
        </div>
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
import { entryApi, eventApi } from '@/services/api'
import {
  PAYMENT_MAX_AGE_MS, backupMatches, claimOnChain, couponId, exportCouponBackup, importCouponBackup, loadChainCoupons,
  loadPending, recoverFromChain, refundOnChain, removePending, revealOnChain, submitPending
} from '@/services/entry'
import { couponPayout, eventPda, fetchEvent, systemFromPicks, walletCoupons } from '@/services/program'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { explorerAddress, explorerTx, formatDate, formatSol, statusColors } from '@/utils/format'

const entries = ref([])
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
let chainContext = { onChain: [], events: new Map() }

function describe (onChain, saved, state, now) {
  const restorable = onChain && !onChain.revealed && backupMatches(auth.wallet, saved, onChain)
  if (!onChain) {
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
      detail: onChain.revealed ? `Results: ${state.results}` : 'The picks were not revealed in time, so the stake stayed in the prize pool.'
    }
  }
  if (state.status === 'tallying') return { label: 'Counting', color: 'primary', detail: 'Results are locked and coupons are being counted.' }
  if (now < state.deadline) return { label: 'Entered', color: 'primary', detail: `Reveal your picks after entries close (${formatDate(state.deadline * 1000)}).` }
  if (onChain.revealed) return { label: 'Revealed', color: 'primary', detail: 'Waiting for both result signers.' }
  if (now >= state.revealDeadline) return { label: 'Not revealed', color: 'error', detail: 'The reveal window has closed; this coupon cannot win.' }
  if (restorable) return { label: 'Reveal now', color: 'warning', action: 'reveal', detail: `Reveal before ${formatDate(state.revealDeadline * 1000)} or the coupon cannot win.` }
  return { label: 'Picks missing', color: 'error', detail: 'This browser does not have the picks. Recover them from chain or import a backup.' }
}

async function loadChain () {
  chainError.value = ''
  await loadConfig()
  const rpcUrl = auth.config.solanaRpcUrl
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
  await Promise.all([...addresses].map(async address => {
    try {
      states.set(address, await fetchEvent(rpcUrl, address))
    } catch {
      // the event row explains that its state is unknown
    }
  }))
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
      admin: event?.chain.admin ?? local?.admin, nonce: event?.chain.nonce ?? local?.nonce,
      system, revealNonce: local?.revealNonce, ...describe(coupon, local, state, now)
    })
  }
  for (const local of localOnly.values()) {
    rows.push({ id: couponId(local), ...local, ...describe(null, local, null, now) })
  }
  unrecovered.value = onChain.filter(coupon => {
    const state = states.get(coupon.event)
    const canReveal = state?.status === 'open' && now < state.revealDeadline
    return canReveal && !coupon.revealed && !backupMatches(auth.wallet, savedById.get(coupon.address), coupon)
  }).length
  chainRows.value = rows
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
const gameLabel = (entry, index) => {
  const game = entry.event?.games?.[index]
  return game ? `${game.homeTeam} vs ${game.awayTeam}` : 'Game'
}
const expired = p => Date.now() - p.paidAt > PAYMENT_MAX_AGE_MS

async function load () {
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

onMounted(load)
</script>

<style scoped>
.game-label { min-width: 180px; }
</style>
