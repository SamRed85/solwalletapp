<template>
  <v-container class="py-8" max-width="900">
    <div class="text-center mb-8">
      <v-icon size="72" color="primary" class="mb-4">mdi-target</v-icon>
      <h1 class="text-h3 font-weight-light mb-2">Welcome to Sol</h1>
      <p class="text-h6 text-medium-emphasis">Predict match outcomes with your Solana wallet</p>
    </div>

    <v-card v-if="!auth.wallet" variant="outlined" class="pa-6 text-center">
      <template v-if="phantomInstalled">
        <v-icon size="48" color="primary" class="mb-4">mdi-wallet</v-icon>
        <h2 class="text-h5 mb-2">Ready to get started?</h2>
        <p class="text-body-1 text-medium-emphasis mb-4">
          Connect Phantom and sign a message to prove you own the wallet. Signing is free and sends no transaction.
        </p>
        <v-btn color="primary" size="large" prepend-icon="mdi-wallet" :loading="auth.busy" @click="onLogin">
          Connect wallet
        </v-btn>
      </template>
      <template v-else>
        <v-icon size="48" color="warning" class="mb-4">mdi-wallet-outline</v-icon>
        <h2 class="text-h5 mb-2">Phantom wallet required</h2>
        <p class="text-body-1 text-medium-emphasis mb-4">
          Install the Phantom browser extension and switch it to Devnet in its settings.
        </p>
        <v-btn color="primary" href="https://phantom.app/" target="_blank" prepend-icon="mdi-download">
          Install Phantom
        </v-btn>
      </template>
    </v-card>

    <template v-else>
      <v-card>
        <v-card-title class="d-flex align-center">
          <v-icon class="mr-2">mdi-calendar</v-icon>
          Events
          <v-spacer />
          <v-btn v-if="auth.isAdmin" variant="text" color="warning" prepend-icon="mdi-shield-crown" to="/admin">
            Manage
          </v-btn>
        </v-card-title>
        <v-progress-linear v-if="loading" indeterminate />
        <v-card-text v-if="!loading && events.length === 0" class="text-center text-medium-emphasis">
          No events yet.
        </v-card-text>
        <v-list v-else lines="two">
          <v-list-item
            v-for="event in events" :key="event.id" :title="event.title" :to="`/events/${event.id}`" lines="three"
            :disabled="event.status === 'draft' && !auth.isAdmin"
            :subtitle="`${event.gameCount} games · ${formatSol(event.couponPriceLamports)} per coupon · entries close ${formatDate(event.startTime)}`"
          >
            <template #append>
              <v-chip v-if="event.rolloverInLamports" color="amber" size="small" variant="flat" class="mr-2" prepend-icon="mdi-trophy">
                +{{ formatSol(event.rolloverInLamports) }}
              </v-chip>
              <v-chip :color="statusColors[event.status]" size="small" variant="flat">
                {{ event.status === 'open' ? 'play' : event.status }}
              </v-chip>
            </template>
          </v-list-item>
        </v-list>
      </v-card>
      <div class="text-center mt-4">
        <v-btn variant="text" prepend-icon="mdi-ticket" to="/coupons">My Coupons</v-btn>
      </div>
    </template>
  </v-container>
</template>

<script setup>
import { ref, watch } from 'vue'
import { eventApi } from '@/services/api'
import { isPhantomInstalled } from '@/services/solana'
import { auth, login } from '@/stores/auth'
import { notifyError } from '@/stores/notify'
import { formatDate, formatSol, statusColors } from '@/utils/format'

const phantomInstalled = isPhantomInstalled()
const events = ref([])
const loading = ref(false)

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

async function onLogin () {
  try {
    await login()
  } catch (error) {
    notifyError(error)
  }
}

watch(() => auth.wallet, wallet => {
  if (wallet) loadEvents()
  else events.value = []
}, { immediate: true })
</script>
