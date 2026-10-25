<template>
  <v-app>
    <v-app-bar color="primary">
      <v-app-bar-nav-icon @click="drawer = !drawer" />
      <v-app-bar-title>Sol</v-app-bar-title>

      <template v-if="auth.wallet">
        <v-chip v-if="auth.balance !== null" class="mr-2 d-none d-sm-flex" size="small" prepend-icon="mdi-wallet" variant="flat" color="secondary">
          {{ auth.balance.toFixed(4) }} SOL
        </v-chip>
        <v-chip
          class="mr-2" size="small" variant="flat" :color="auth.isAdmin ? 'warning' : 'success'"
          :prepend-icon="auth.isAdmin ? 'mdi-shield-crown' : 'mdi-account'"
        >
          {{ auth.displayName || shortAddress(auth.wallet) }}
        </v-chip>
        <v-btn prepend-icon="mdi-logout" @click="onLogout">Logout</v-btn>
      </template>
      <v-btn v-else variant="outlined" prepend-icon="mdi-wallet" :loading="auth.busy" class="mr-2" @click="onLogin">
        Connect
      </v-btn>
    </v-app-bar>

    <v-navigation-drawer v-model="drawer" temporary>
      <v-list nav>
        <v-list-item v-for="item in menuItems" :key="item.to" :to="item.to" :prepend-icon="item.icon" :title="item.title" />
      </v-list>
    </v-navigation-drawer>

    <v-main>
      <router-view />
    </v-main>

    <v-footer app height="40" class="text-caption">
      <v-icon :color="healthy ? 'success' : 'error'" size="small" class="mr-1">
        {{ healthy ? 'mdi-check-circle' : 'mdi-alert-circle' }}
      </v-icon>
      Backend: {{ healthy === null ? 'checking...' : healthy ? 'healthy' : 'unreachable' }}
      <v-chip size="x-small" color="warning" variant="tonal" class="ml-3">devnet</v-chip>
      <v-spacer />
      <v-btn size="small" variant="text" :prepend-icon="isDark ? 'mdi-weather-night' : 'mdi-weather-sunny'" @click="toggleTheme">
        {{ isDark ? 'Dark' : 'Light' }}
      </v-btn>
    </v-footer>

    <v-dialog :model-value="needsName" persistent max-width="480">
      <v-card title="Choose your profile name">
        <v-card-text>
          <p class="mb-4">This is the first time this wallet has signed in. Pick the name other people will see. You can change it later from your profile. Your wallet address stays the same.</p>
          <v-text-field v-model="newName" label="Profile name" :rules="[nameRule]" counter="32" maxlength="32" autofocus @keyup.enter="saveName" />
        </v-card-text>
        <v-card-actions>
          <v-btn variant="text" @click="onLogout">Log out</v-btn>
          <v-spacer />
          <v-btn color="primary" variant="flat" :disabled="nameRule(newName) !== true" :loading="savingName" @click="saveName">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar v-model="snackbar.show" :color="snackbar.color" location="top" timeout="5000">
      {{ snackbar.text }}
      <template #actions>
        <v-btn variant="text" @click="snackbar.show = false">Close</v-btn>
      </template>
    </v-snackbar>
  </v-app>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTheme } from 'vuetify'
import { authApi, healthApi } from '@/services/api'
import { startAutoReveal } from '@/services/autoReveal'
import { auth, login, logout, setProfile } from '@/stores/auth'
import { notify, notifyError, snackbar } from '@/stores/notify'
import { shortAddress } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const theme = useTheme()
const drawer = ref(false)
const healthy = ref(null)
const newName = ref('')
const savingName = ref(false)
const needsName = computed(() => !!auth.wallet && !auth.displayName)
const nameRule = value => {
  const name = value.trim().replace(/\s+/g, ' ')
  return name.length >= 2 && name.length <= 32 ? true : 'Use 2 to 32 characters'
}

const isDark = computed(() => theme.global.current.value.dark)

const menuItems = computed(() => [
  { title: 'Home', icon: 'mdi-home', to: '/' },
  ...(auth.wallet
    ? [
        { title: 'My Coupons', icon: 'mdi-ticket', to: '/coupons' },
        { title: 'Profile', icon: 'mdi-account', to: '/profile' }
      ]
    : []),
  ...(auth.isAdmin
    ? [
        { title: 'Admin', icon: 'mdi-shield-crown', to: '/admin' },
        { title: 'Users', icon: 'mdi-account-group', to: '/users' }
      ]
    : [])
])

watch(() => auth.isAdmin, isAdmin => {
  if (!isAdmin && route.meta.requiresAdmin) router.push('/')
})

watch(() => auth.wallet, wallet => {
  if (!wallet && route.meta.requiresAuth) router.push('/')
})

function toggleTheme () {
  const next = isDark.value ? 'light' : 'dark'
  theme.change(next)
  localStorage.setItem('theme', next)
}

async function onLogin () {
  try {
    await login()
    notify('Signed in')
  } catch (error) {
    notifyError(error)
  }
}

async function saveName () {
  if (nameRule(newName.value) !== true) return
  savingName.value = true
  try {
    setProfile(await authApi.updateProfile(newName.value))
    newName.value = ''
    notify('Profile name saved')
  } catch (error) {
    notifyError(error)
  } finally {
    savingName.value = false
  }
}

async function onLogout () {
  await logout()
  notify('Signed out', 'info')
}

async function checkHealth () {
  try {
    healthy.value = (await healthApi.check()).status === 'healthy'
  } catch {
    healthy.value = false
  }
}

let healthTimer
onMounted(() => {
  startAutoReveal()
  checkHealth()
  healthTimer = setInterval(checkHealth, 30000)
})
onBeforeUnmount(() => {
  clearInterval(healthTimer)
})
</script>
