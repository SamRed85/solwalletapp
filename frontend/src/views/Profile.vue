<template>
  <v-container class="py-6" max-width="640">
    <v-card>
      <v-card-title>Profile</v-card-title>
      <v-card-text>
        <p class="text-medium-emphasis mb-4">Your wallet address cannot be changed. The profile name is what the app shows for you.</p>
        <div class="text-caption text-medium-emphasis">Wallet</div>
        <code class="d-block mb-4">{{ auth.wallet }}</code>
        <v-text-field v-model="name" label="Profile name" :rules="[nameRule]" counter="32" maxlength="32" />
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
import { authApi } from '@/services/api'
import { auth, setProfile } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'

const name = ref(auth.displayName)
const saving = ref(false)

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
