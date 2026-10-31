<template>
  <v-container class="py-6" max-width="640">
    <v-card>
      <v-card-title>Profile</v-card-title>
      <v-card-text>
        <p class="text-medium-emphasis mb-4">Your wallet address cannot be changed. The profile name is what the app shows for you.</p>
        <div v-if="auth.balance !== null" class="mb-4">
          <div class="text-caption text-medium-emphasis">Balance</div>
          <div class="text-h6"><SolAmount :lamports="Math.round(auth.balance * 1e9)" /></div>
        </div>
        <div class="text-caption text-medium-emphasis">Wallet</div>
        <code class="wallet d-block mb-4">{{ auth.wallet }}</code>
        <v-text-field v-model="name" label="Profile name" :rules="[nameRule]" counter="32" maxlength="32" />
        <div class="text-caption text-medium-emphasis mt-2">Display currency</div>
        <p class="text-medium-emphasis mb-3">This only changes prices on screen. Coupons are still paid and settled in SOL.</p>
        <v-btn-toggle v-model="currency" class="currency-toggle" mandatory divided color="primary">
          <v-btn value="SOL">SOL</v-btn>
          <v-btn value="USD">USD</v-btn>
          <v-btn value="SEK">SEK</v-btn>
        </v-btn-toggle>
        <p v-if="rateLine" class="text-caption text-medium-emphasis mt-2 mb-0">{{ rateLine }}</p>
        <p v-else-if="display.rateError && currency !== 'SOL'" class="text-caption mt-2 mb-0">The SOL price could not be loaded, so amounts stay in SOL.</p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn color="primary" variant="flat" :disabled="!changed" :loading="saving" @click="save">Save</v-btn>
      </v-card-actions>
    </v-card>
  </v-container>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import SolAmount from '@/components/SolAmount.vue'
import { authApi } from '@/services/api'
import { auth, setProfile } from '@/stores/auth'
import { display, setCurrency } from '@/stores/currency'
import { notify, notifyError } from '@/stores/notify'

const name = ref(auth.displayName)
const saving = ref(false)
const currency = computed({
  get: () => display.currency,
  set: setCurrency
})
const rateLine = computed(() => {
  if (!display.rates) return ''
  const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(display.rates.usd)
  const sek = new Intl.NumberFormat('sv-SE', { style: 'currency', currency: 'SEK', maximumFractionDigits: 2 }).format(display.rates.sek)
  return `About ${usd} · ${sek} per SOL`
})

watch(() => auth.displayName, value => {
  name.value = value
})

const normalized = value => value.trim().replace(/\s+/g, ' ')
const nameRule = value => {
  const next = normalized(value || '')
  return next.length >= 2 && next.length <= 32 ? true : 'Use 2 to 32 characters'
}
const changed = computed(() => nameRule(name.value) === true && normalized(name.value) !== auth.displayName)

async function save () {
  saving.value = true
  try {
    setProfile(await authApi.updateProfile(name.value))
    notify('Profile name saved')
  } catch (error) {
    notifyError(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.wallet { word-break: break-all; }
.currency-toggle { display: flex; width: 100%; }
.currency-toggle :deep(.v-btn) { flex: 1; }
</style>
