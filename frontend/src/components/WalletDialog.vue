<template>
  <v-dialog :model-value="walletChoices.open" max-width="420" @update:model-value="walletChoices.open = $event">
    <v-card title="Connect a wallet">
      <v-card-text>
        <p class="intro">
          Phantom, Jupiter, Solflare, or Backpack. Signing is free and sends no transaction. Set the wallet to Devnet.
          On a phone, open this page in the wallet's browser. On Android, Jupiter Mobile can also connect from Chrome.
        </p>
        <div class="choices">
          <button v-for="item in walletChoices.list" :key="item.name" type="button" class="choice" :disabled="auth.busy" @click="pick(item.name)">
            <img v-if="item.icon" :src="item.icon" alt="" width="28" height="28">
            <v-icon v-else icon="mdi-wallet" />
            <span>{{ choiceLabel(item) }}</span>
          </button>
          <a v-for="item in missing" :key="item.kind" class="choice" :href="item.href" target="_blank" rel="noopener noreferrer">
            <v-icon icon="mdi-download" />
            <span>Get {{ item.label }}</span>
          </a>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" :disabled="auth.busy" @click="walletChoices.open = false">Close</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { auth, login, walletChoices } from '@/stores/auth'
import { choiceLabel, missingWallets, walletByName } from '@/services/wallets'
import { notify, notifyError } from '@/stores/notify'

const phone = /Android|iPhone|iPad|iPod/i.test(navigator.userAgent)
const missing = computed(() => missingWallets(phone))

async function pick (name) {
  const wallet = walletByName(name)
  if (!wallet) return
  try {
    await login(wallet)
    notify('Signed in')
  } catch (error) {
    notifyError(error)
  }
}
</script>

<style scoped>
.intro { margin: 0 0 16px; color: rgba(var(--v-theme-on-surface), 0.62); }
.choices { display: flex; flex-direction: column; gap: 8px; }
.choice {
  display: flex; align-items: center; gap: 12px; width: 100%; min-height: 52px; padding: 0 14px;
  border: 0; border-radius: 14px; background: rgb(var(--v-theme-surface-variant));
  color: rgb(var(--v-theme-on-surface)); font: inherit; font-weight: 600; text-align: left;
  text-decoration: none; cursor: pointer;
}
.choice:disabled { opacity: 0.6; cursor: default; }
.choice img { width: 28px; height: 28px; border-radius: 8px; object-fit: contain; }
</style>
