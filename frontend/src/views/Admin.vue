<template>
  <v-container class="py-6">
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon color="warning" class="mr-2">mdi-shield-crown</v-icon>
        Events
        <v-spacer />
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">Create event</v-btn>
      </v-card-title>
      <v-card-subtitle>
        Games can be set once while the event is a draft. Results can be set once, after the start time.
      </v-card-subtitle>

      <v-data-table :headers="headers" :items="events" :loading="loading" item-value="id">
        <template #[`item.status`]="{ value }">
          <v-chip :color="statusColors[value]" size="small" variant="flat">{{ value }}</v-chip>
        </template>
        <template #[`item.startTime`]="{ value }">{{ formatDate(value) }}</template>
        <template #[`item.actions`]="{ item }">
          <v-btn icon="mdi-eye" size="small" variant="text" title="Details" @click="openDetails(item)" />
          <v-btn
            v-if="item.status === 'completed'" icon="mdi-cash-multiple" size="small" variant="text"
            color="success" title="Payouts" @click="openDetails(item)"
          />
          <v-btn
            v-if="item.status === 'open' && !item.chain" icon="mdi-safe" size="small" variant="text"
            color="primary" title="Open the on-chain vault" :loading="publishing === item.id" @click="publishVault(item)"
          />
          <v-btn
            v-if="item.status === 'draft'" icon="mdi-format-list-numbered" size="small" variant="text"
            color="success" title="Set games" @click="openGames(item)"
          />
          <v-btn
            v-if="item.status === 'closed'" icon="mdi-flag-checkered" size="small" variant="text"
            color="warning" title="Set results" @click="openResults(item)"
          />
        </template>
      </v-data-table>
    </v-card>

    <v-card class="mt-4">
      <v-card-title>Payment recoveries</v-card-title>
      <v-card-subtitle>Finalized payments that did not become coupons. Refunds are verified on devnet before being marked complete.</v-card-subtitle>
      <v-card-text>
        <v-alert v-if="recoveries.length && auth.wallet !== config?.treasuryWallet" type="warning" variant="tonal" class="mb-3">
          Connect the configured treasury wallet to send these refunds.
        </v-alert>
        <p v-if="recoveries.length === 0" class="text-medium-emphasis">No refunds are waiting.</p>
        <v-table v-else density="compact">
          <thead><tr><th>Wallet</th><th>Event</th><th>Amount</th><th /></tr></thead>
          <tbody>
            <tr v-for="recovery in recoveries" :key="recovery.signature">
              <td><code>{{ recovery.walletAddress }}</code></td>
              <td><code>{{ recovery.eventId }}</code></td>
              <td>{{ formatSol(recovery.lamports) }}</td>
              <td class="text-right">
                <v-btn
                  color="warning" size="small" variant="flat"
                  :disabled="auth.wallet !== config?.treasuryWallet"
                  :loading="refunding === recovery.signature"
                  @click="refundRecovery(recovery)"
                >
                  Refund
                </v-btn>
              </td>
            </tr>
          </tbody>
        </v-table>
      </v-card-text>
    </v-card>

    <v-dialog v-model="createDialog" max-width="600">
      <v-card title="Create event">
        <v-card-text>
          <v-form ref="createForm" v-model="createValid">
            <v-text-field v-model="newEvent.title" label="Title" :rules="[required, v => v.length <= 120 || 'Max 120 characters']" />
            <v-textarea v-model="newEvent.description" label="Description" rows="3" :rules="[v => v.length <= 1000 || 'Max 1000 characters']" />
            <v-text-field
              v-model.number="newEvent.gameCount" label="Number of games" type="number" min="7" max="13"
              :rules="[v => (v >= 7 && v <= 13) || 'Between 7 and 13 games']"
              :hint="isSpecial ? 'Special event: set the price and prize tiers below' : 'Standard event'" persistent-hint
              @update:model-value="resetTiers"
            />
            <v-switch
              v-if="!isSpecial" v-model="newEvent.customRules" label="Custom price and prize tiers"
              color="primary" density="compact" hide-details class="mt-2" @update:model-value="resetTiers"
            />
            <template v-if="isSpecial || newEvent.customRules">
              <v-text-field
                v-model.number="newEvent.priceSol" label="Coupon price (SOL)" type="number" step="0.001" min="0" class="mt-4"
                :rules="[v => (v > 0 && v <= 10) || 'Between 0 and 10 SOL']"
              />
              <div class="text-subtitle-2 mb-2">
                Prize tiers ({{ tierTotal }}% of {{ config?.tierPercentTotal ?? 95 }}% allocated, the house keeps {{ config?.houseCutPercent ?? 5 }}%)
              </div>
              <v-row v-for="(tier, index) in newEvent.tiers" :key="index" dense align="center">
                <v-col cols="5">
                  <v-text-field v-model.number="tier.correct" label="Correct picks" type="number" density="compact" hide-details :disabled="index === 0" />
                </v-col>
                <v-col cols="5">
                  <v-text-field v-model.number="tier.percent" label="Percent" type="number" suffix="%" density="compact" hide-details />
                </v-col>
                <v-col cols="2">
                  <v-btn v-if="index > 0" icon="mdi-delete" size="small" variant="text" @click="newEvent.tiers.splice(index, 1)" />
                </v-col>
              </v-row>
              <v-btn size="small" variant="text" prepend-icon="mdi-plus" class="mt-1" @click="newEvent.tiers.push({ correct: newEvent.gameCount - newEvent.tiers.length, percent: 0 })">
                Add tier
              </v-btn>
              <v-alert v-if="tierError" type="error" variant="tonal" density="compact" class="mt-2">{{ tierError }}</v-alert>
            </template>
            <v-text-field
              v-model="newEvent.startTime" label="Start time (entries close)" type="datetime-local"
              :rules="[required, v => new Date(v) > new Date() || 'Must be in the future']"
            />
            <v-text-field
              v-model="newEvent.endTime" label="End time" type="datetime-local"
              :rules="[required, v => new Date(v) > new Date(newEvent.startTime) || 'Must be after start time']"
            />
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="createDialog = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!createValid || !!tierError" :loading="saving" @click="createEvent">Create</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="gamesDialog" max-width="800" scrollable>
      <v-card :title="`Games for ${selected?.title}`">
        <v-card-text>
          <v-alert type="warning" variant="tonal" density="compact" class="mb-4">
            Games cannot be changed after they are saved.
          </v-alert>
          <v-form v-model="gamesValid">
            <p class="text-caption text-medium-emphasis mb-2">
              Kickoffs must be between the event start ({{ formatDate(selected?.startTime) }}) and end. Entries close at the event start.
            </p>
            <v-row v-for="(game, index) in newGames" :key="index" dense align="center">
              <v-col cols="1" class="text-medium-emphasis">{{ index + 1 }}.</v-col>
              <v-col cols="4"><v-text-field v-model="game.homeTeam" label="Home team" density="compact" :rules="[required]" /></v-col>
              <v-col cols="4"><v-text-field v-model="game.awayTeam" label="Away team" density="compact" :rules="[required]" /></v-col>
              <v-col cols="3"><v-text-field v-model="game.kickoff" label="Kickoff" type="datetime-local" density="compact" :rules="[required]" /></v-col>
            </v-row>
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="gamesDialog = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!gamesValid" :loading="saving" @click="saveGames">Save games</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="resultsDialog" max-width="800" scrollable>
      <v-card :title="`Results for ${selected?.title}`">
        <v-card-text>
          <v-alert type="warning" variant="tonal" density="compact" class="mb-4">
            Results cannot be changed after they are saved.
          </v-alert>
          <p class="text-caption text-medium-emphasis mb-4">
            For a cancelled or postponed game choose Cancelled. Its result is drawn at random, weighted by how the
            coupons picked it, from the first devnet block 1 hour after kickoff. This is allowed from then on.
          </p>
          <div v-for="(game, index) in selected?.games" :key="index" class="mb-2">
            <div class="text-subtitle-1">{{ index + 1 }}. {{ game.homeTeam }} vs {{ game.awayTeam }}</div>
            <v-radio-group v-model="results[index]" inline hide-details>
              <v-radio v-for="o in outcomes" :key="o.value" :label="o.label" :value="o.value" />
              <v-radio label="Cancelled" value="C" color="warning" :disabled="!canDraw(game)" />
            </v-radio-group>
            <div v-if="!canDraw(game)" class="text-caption text-medium-emphasis">
              Can be marked cancelled from {{ formatDate(drawTime(game)) }}
            </div>
          </div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="resultsDialog = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!resultsComplete" :loading="saving" @click="saveResults">
            Save results
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog :model-value="!!details" max-width="700" scrollable @update:model-value="details = null">
      <v-card v-if="details" :title="details.title" :subtitle="details.description">
        <v-card-text>
          <p><strong>Status:</strong> {{ details.status }}</p>
          <p><strong>Start (entries close):</strong> {{ formatDate(details.startTime) }}</p>
          <p><strong>End:</strong> {{ formatDate(details.endTime) }}</p>
          <p><strong>Coupon price:</strong> {{ formatSol(details.couponPriceLamports) }} · <strong>Coupons sold:</strong> {{ details.couponsSold }}</p>
          <p><strong>Tiers:</strong> {{ details.tiers.map(t => `${t.correct} correct ${t.percent}%`).join(', ') }}</p>
          <p v-if="details.rolloverInLamports"><strong>Rollover received:</strong> {{ formatSol(details.rolloverInLamports) }}</p>
          <p class="mb-4"><strong>Created by:</strong> <code>{{ details.createdBy }}</code></p>
          <p v-if="details.games.length === 0" class="text-medium-emphasis">No games yet.</p>
          <v-table v-else density="compact">
            <thead><tr><th>#</th><th>Home</th><th>Away</th><th>Kickoff</th><th>Result</th></tr></thead>
            <tbody>
              <tr v-for="(game, index) in details.games" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ game.homeTeam }}</td>
                <td>{{ game.awayTeam }}</td>
                <td>{{ formatDate(game.kickoff) }}</td>
                <td>
                  {{ outcomeLabels[game.result] || '—' }}
                  <v-chip v-if="game.cancelled" size="x-small" color="warning" class="ml-1">drawn</v-chip>
                </td>
              </tr>
            </tbody>
          </v-table>

          <template v-if="details.settlement">
            <div class="text-subtitle-1 mt-4">Settlement</div>
            <p>
              Sales {{ formatSol(details.settlement.salesLamports) }} · rollover in {{ formatSol(details.settlement.rolloverInLamports) }} ·
              house {{ formatSol(details.settlement.houseLamports) }} · to winners {{ formatSol(details.settlement.paidLamports) }} ·
              rollover out {{ formatSol(details.settlement.rolloverOutLamports) }}
            </p>
            <div class="text-subtitle-1 mt-4">Payouts to send from the treasury</div>
            <v-progress-linear v-if="!payouts" indeterminate />
            <p v-else-if="payouts.length === 0" class="text-medium-emphasis">No winners in this event.</p>
            <v-table v-else density="compact">
              <thead><tr><th>Wallet</th><th>Winning coupons</th><th>Amount</th></tr></thead>
              <tbody>
                <tr v-for="p in payouts" :key="p.walletAddress">
                  <td><code>{{ p.walletAddress }}</code></td>
                  <td>{{ p.winningCoupons }}</td>
                  <td>{{ formatSol(p.winningsLamports) }}</td>
                </tr>
              </tbody>
            </v-table>
          </template>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="details = null">Close</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { eventApi, recoveryApi } from '@/services/api'
import { PROGRAM_ID, RESULT_COSIGNER, createEventIx, sendProgramTransaction, vaultSchedule } from '@/services/program'
import { sendPayment, waitForConfirmation } from '@/services/solana'
import { auth, loadConfig } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { formatDate, formatSol, outcomeLabels, outcomes, statusColors, toLocalInput } from '@/utils/format'

const DRAW_DELAY_MS = 60 * 60 * 1000

const headers = [
  { title: 'Title', key: 'title' },
  { title: 'Status', key: 'status' },
  { title: 'Games', key: 'gameCount' },
  { title: 'Entries close', key: 'startTime' },
  { title: '', key: 'actions', sortable: false, align: 'end' }
]
const required = v => !!v?.trim?.() || 'Required'

const events = ref([])
const loading = ref(false)
const saving = ref(false)
const selected = ref(null)
const details = ref(null)
const payouts = ref(null)
const config = computed(() => auth.config)
const recoveries = ref([])
const refunding = ref(null)
const publishing = ref(null)

const createDialog = ref(false)
const createValid = ref(false)
const newEvent = ref({})
const defaultGameCount = computed(() => config.value?.defaultGameCount ?? 13)
const isSpecial = computed(() => newEvent.value.gameCount !== defaultGameCount.value)
const tierTotal = computed(() => (newEvent.value.tiers ?? []).reduce((sum, t) => sum + (Number(t.percent) || 0), 0))
const tierError = computed(() => {
  if (!isSpecial.value && !newEvent.value.customRules) return ''
  const tiers = newEvent.value.tiers ?? []
  const counts = tiers.map(t => t.correct)
  if (tiers.some(t => !(t.correct >= 1 && t.correct <= newEvent.value.gameCount))) return 'Correct picks must be between 1 and the number of games'
  if (new Set(counts).size !== counts.length) return 'Each number of correct picks can only appear once'
  if (tiers.some(t => !(t.percent >= 1))) return 'Every tier needs a positive percentage'
  const target = config.value?.tierPercentTotal ?? 95
  if (tierTotal.value !== target) return `Tiers must add up to ${target}%`
  return ''
})

// Standard split (50/25/15/5) shifted to the chosen number of games; the admin can adjust it.
function resetTiers () {
  const n = newEvent.value.gameCount
  const percents = (config.value?.defaultTiers ?? []).map(t => t.percent)
  newEvent.value.tiers = percents.map((percent, i) => ({ correct: n - i, percent })).filter(t => t.correct > 0)
}

const gamesDialog = ref(false)
const gamesValid = ref(false)
const newGames = ref([])

const resultsDialog = ref(false)
const results = ref([])
const resultsComplete = computed(() =>
  selected.value && results.value.filter(Boolean).length === selected.value.games.length)

async function loadEvents () {
  loading.value = true
  try {
    events.value = await eventApi.list()
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

async function publishVault (item) {
  publishing.value = item.id
  try {
    const schedule = vaultSchedule(item)
    const nonce = Date.now()
    const sent = await sendProgramTransaction(config.value.solanaRpcUrl, auth.wallet, [
      createEventIx({
        admin: auth.wallet,
        other: RESULT_COSIGNER,
        nonce,
        ...schedule,
        gameCount: item.gameCount,
        price: item.couponPriceLamports,
        tiers: item.tiers
      })
    ])
    await waitForConfirmation(config.value.solanaRpcUrl, sent.signature, sent.lastValidBlockHeight)
    await eventApi.setChain(item.id, { programId: PROGRAM_ID, admin: auth.wallet, cosigner: RESULT_COSIGNER, nonce })
    notify('On-chain vault opened. Entries now pay the program, not the treasury.')
    await loadEvents()
  } catch (error) {
    notifyError(error)
  } finally {
    publishing.value = null
  }
}

async function loadRecoveries () {
  try {
    recoveries.value = await recoveryApi.list()
  } catch (error) {
    notifyError(error)
  }
}

async function refundRecovery (recovery) {
  refunding.value = recovery.signature
  try {
    const memo = `sol-refund-v1:${recovery.signature}`
    const sent = await sendPayment(config.value.solanaRpcUrl, auth.wallet, recovery.walletAddress, recovery.lamports, memo)
    await waitForConfirmation(config.value.solanaRpcUrl, sent.signature, sent.lastValidBlockHeight)
    await recoveryApi.refund(recovery.signature, sent.signature)
    notify('Refund finalized and recorded')
    await loadRecoveries()
  } catch (error) {
    notifyError(error)
  } finally {
    refunding.value = null
  }
}

async function save (action, successMessage, closeDialog) {
  saving.value = true
  try {
    await action()
    notify(successMessage)
    closeDialog()
    await loadEvents()
  } catch (error) {
    notifyError(error)
  } finally {
    saving.value = false
  }
}

async function openCreate () {
  try {
    await loadConfig()
  } catch (error) {
    notifyError(error)
    return
  }
  newEvent.value = {
    title: '',
    description: '',
    gameCount: defaultGameCount.value,
    startTime: '',
    endTime: '',
    customRules: false,
    priceSol: config.value.couponPriceLamports / 1e9,
    tiers: []
  }
  resetTiers()
  createDialog.value = true
}

function createEvent () {
  const { title, description, gameCount, startTime, endTime, customRules, priceSol, tiers } = newEvent.value
  const body = {
    title,
    description,
    gameCount,
    startTime: new Date(startTime).toISOString(),
    endTime: new Date(endTime).toISOString()
  }
  if (isSpecial.value || customRules) {
    body.couponPriceLamports = Math.round(priceSol * 1e9)
    body.tiers = tiers.map(t => ({ correct: Number(t.correct), percent: Number(t.percent) }))
  }
  return save(() => eventApi.create(body), 'Event created', () => { createDialog.value = false })
}

function openGames (event) {
  selected.value = event
  const kickoff = toLocalInput(event.startTime)
  newGames.value = Array.from({ length: event.gameCount }, () => ({ homeTeam: '', awayTeam: '', kickoff }))
  gamesDialog.value = true
}

const saveGames = () => save(
  () => eventApi.setGames(selected.value.id, newGames.value.map(g => ({ ...g, kickoff: new Date(g.kickoff).toISOString() }))),
  'Games saved, the event is now open for entries',
  () => { gamesDialog.value = false }
)

const drawTime = game => new Date(new Date(game.kickoff).getTime() + DRAW_DELAY_MS)
const canDraw = game => Date.now() >= drawTime(game).getTime()

function openResults (event) {
  selected.value = event
  results.value = event.games.map(() => null)
  resultsDialog.value = true
}

const saveResults = () => save(
  () => eventApi.setResults(selected.value.id, results.value),
  'Results saved and the event is settled',
  () => { resultsDialog.value = false }
)

async function openDetails (event) {
  details.value = event
  payouts.value = null
  if (!event.settlement) return
  try {
    payouts.value = (await eventApi.payouts(event.id)).payouts
  } catch (error) {
    notifyError(error)
  }
}

onMounted(() => {
  loadEvents()
  loadRecoveries()
  loadConfig().catch(notifyError)
})
</script>
