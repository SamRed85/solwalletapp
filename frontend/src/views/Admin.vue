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
          <v-btn icon="mdi-eye" size="small" variant="text" title="Details" @click="details = item" />
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

    <v-dialog v-model="createDialog" max-width="600">
      <v-card title="Create event">
        <v-card-text>
          <v-form ref="createForm" v-model="createValid">
            <v-text-field v-model="newEvent.title" label="Title" :rules="[required, v => v.length <= 120 || 'Max 120 characters']" />
            <v-textarea v-model="newEvent.description" label="Description" rows="3" :rules="[v => v.length <= 1000 || 'Max 1000 characters']" />
            <v-text-field
              v-model.number="newEvent.gameCount" label="Number of games" type="number" min="7" max="13"
              :rules="[v => (v >= 7 && v <= 13) || 'Between 7 and 13 games']"
            />
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
          <v-btn color="primary" variant="flat" :disabled="!createValid" :loading="saving" @click="createEvent">Create</v-btn>
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
            <v-row v-for="(game, index) in newGames" :key="index" dense align="center">
              <v-col cols="1" class="text-medium-emphasis">{{ index + 1 }}.</v-col>
              <v-col cols="5"><v-text-field v-model="game.homeTeam" label="Home team" density="compact" :rules="[required]" /></v-col>
              <v-col cols="1" class="text-center">vs</v-col>
              <v-col cols="5"><v-text-field v-model="game.awayTeam" label="Away team" density="compact" :rules="[required]" /></v-col>
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
          <div v-for="(game, index) in selected?.games" :key="index" class="mb-2">
            <div class="text-subtitle-1">{{ index + 1 }}. {{ game.homeTeam }} vs {{ game.awayTeam }}</div>
            <v-radio-group v-model="results[index]" inline hide-details>
              <v-radio v-for="o in outcomes" :key="o.value" :label="o.label" :value="o.value" />
            </v-radio-group>
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
          <p class="mb-4"><strong>Created by:</strong> <code>{{ details.createdBy }}</code></p>
          <p v-if="details.games.length === 0" class="text-medium-emphasis">No games yet.</p>
          <v-table v-else density="compact">
            <thead><tr><th>#</th><th>Home</th><th>Away</th><th>Result</th></tr></thead>
            <tbody>
              <tr v-for="(game, index) in details.games" :key="index">
                <td>{{ index + 1 }}</td>
                <td>{{ game.homeTeam }}</td>
                <td>{{ game.awayTeam }}</td>
                <td>{{ outcomeLabels[game.result] || '—' }}</td>
              </tr>
            </tbody>
          </v-table>
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
import { eventApi } from '@/services/api'
import { notify, notifyError } from '@/stores/notify'
import { formatDate, outcomeLabels, outcomes, statusColors } from '@/utils/format'

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

const createDialog = ref(false)
const createValid = ref(false)
const newEvent = ref({})

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

function openCreate () {
  newEvent.value = { title: '', description: '', gameCount: 7, startTime: '', endTime: '' }
  createDialog.value = true
}

const createEvent = () => save(
  () => eventApi.create({
    ...newEvent.value,
    startTime: new Date(newEvent.value.startTime).toISOString(),
    endTime: new Date(newEvent.value.endTime).toISOString()
  }),
  'Event created',
  () => { createDialog.value = false }
)

function openGames (event) {
  selected.value = event
  newGames.value = Array.from({ length: event.gameCount }, () => ({ homeTeam: '', awayTeam: '' }))
  gamesDialog.value = true
}

const saveGames = () => save(
  () => eventApi.setGames(selected.value.id, newGames.value),
  'Games saved, the event is now open for entries',
  () => { gamesDialog.value = false }
)

function openResults (event) {
  selected.value = event
  results.value = event.games.map(() => null)
  resultsDialog.value = true
}

const saveResults = () => save(
  () => eventApi.setResults(selected.value.id, results.value),
  'Results saved',
  () => { resultsDialog.value = false }
)

onMounted(loadEvents)
</script>
