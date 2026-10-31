<template>
  <v-app>
    <v-app-bar class="sol-bar" :class="{ 'is-phone': mobile }" color="primary" elevation="0">
      <v-app-bar-title class="wordmark">
        Sol
        <v-icon
          v-if="mobile" class="health-dot" size="8"
          :color="healthy === false ? 'error' : 'success'"
          :title="healthy === null ? 'Checking the backend' : healthy ? 'Backend healthy' : 'Backend unreachable'"
        >mdi-circle</v-icon>
      </v-app-bar-title>

      <nav v-show="!mobile" class="bar-nav" aria-label="Pages">
        <router-link
          v-for="item in menuItems"
          :key="item.to"
          :to="item.to"
          class="bar-link"
          active-class=""
          exact-active-class="is-here"
        >
          <v-icon :icon="item.icon" size="small" />
          {{ item.title }}
        </router-link>
      </nav>

      <div class="bar-end">
        <v-btn
          v-if="mobile" icon variant="text"
          :aria-label="isDark ? 'Switch to light' : 'Switch to dark'"
          @click="toggleTheme"
        >
          <v-icon>{{ isDark ? 'mdi-weather-night' : 'mdi-weather-sunny' }}</v-icon>
        </v-btn>
        <template v-if="auth.wallet">
          <v-chip v-if="!mobile && auth.balance !== null" class="mr-2" size="small" prepend-icon="mdi-wallet" variant="flat" color="secondary">
            <SolAmount :lamports="Math.round(auth.balance * 1e9)" />
          </v-chip>
          <v-chip
            to="/profile"
            class="name-chip mr-2" size="small" variant="flat" :color="auth.isAdmin ? 'warning' : 'success'"
            :prepend-icon="auth.isAdmin ? 'mdi-shield-crown' : 'mdi-account'"
            title="Profile"
          >
            {{ auth.displayName || shortAddress(auth.wallet) }}
          </v-chip>
          <v-btn v-if="mobile" icon="mdi-logout" aria-label="Logout" @click="onLogout" />
          <v-btn v-else prepend-icon="mdi-logout" @click="onLogout">Logout</v-btn>
        </template>
        <v-btn v-else variant="outlined" prepend-icon="mdi-wallet" :loading="auth.busy" @click="onLogin">
          Connect
        </v-btn>
      </div>
    </v-app-bar>

    <v-main :class="{ 'has-phone-nav': mobile && menuItems.length > 1 }">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.path" />
        </transition>
      </router-view>
    </v-main>

    <v-bottom-navigation v-if="mobile && menuItems.length > 1" :model-value="navValue" class="phone-nav" grow color="white">
      <v-btn v-for="item in menuItems" :key="item.to" :value="item.to" :to="item.to">
        <v-icon :icon="item.icon" />
        <span>{{ navLabel(item.title) }}</span>
      </v-btn>
    </v-bottom-navigation>

    <v-footer v-if="!mobile" app height="40" class="text-caption">
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
import { useDisplay, useTheme } from 'vuetify'
import { authApi, eventApi, healthApi } from '@/services/api'
import { featuredEvent } from '@/utils/featuredEvent'
import { startAutoReveal } from '@/services/autoReveal'
import { auth, login, logout, setProfile } from '@/stores/auth'
import { loadRates } from '@/stores/currency'
import { notify, notifyError, snackbar } from '@/stores/notify'
import { shortAddress } from '@/utils/format'
import SolAmount from '@/components/SolAmount.vue'

const route = useRoute()
const router = useRouter()
const theme = useTheme()
const { smAndDown: mobile } = useDisplay()
const healthy = ref(null)
const newName = ref('')
const savingName = ref(false)
const needsName = computed(() => !!auth.wallet && !auth.displayName)
const nameRule = value => {
  const name = value.trim().replace(/\s+/g, ' ')
  return name.length >= 2 && name.length <= 32 ? true : 'Use 2 to 32 characters'
}

const isDark = computed(() => theme.global.current.value.dark)
const featured = ref(null)

const menuItems = computed(() => [
  { title: 'Home', icon: 'mdi-home', to: '/' },
  ...(featured.value ? [featured.value] : []),
  ...(auth.wallet
    ? [{ title: 'My Coupons', icon: 'mdi-ticket', to: '/coupons' }]
    : []),
  ...(auth.isAdmin
    ? [
        { title: 'Admin', icon: 'mdi-shield-crown', to: '/admin' },
        { title: 'Users', icon: 'mdi-account-group', to: '/users' }
      ]
    : [])
])

const navValue = computed(() => {
  const exact = menuItems.value.find(item => item.to === route.path)
  if (exact) return exact.to
  const featuredTo = featured.value?.to
  if (featuredTo && route.path === featuredTo) return featuredTo
  return route.path
})

function navLabel (title) {
  if (title === 'Next event') return 'Next'
  if (title === 'Active event') return 'Live'
  if (title === 'My Coupons') return 'Coupons'
  return title
}

watch(() => auth.isAdmin, isAdmin => {
  if (!isAdmin && route.meta.requiresAdmin) router.push('/')
})

watch(() => auth.wallet, wallet => {
  if (!wallet && route.meta.requiresAuth) router.push('/')
  loadFeatured()
})

watch(() => route.path, () => {
  loadFeatured()
})

async function loadFeatured () {
  try {
    featured.value = featuredEvent(await eventApi.list())
  } catch {
    featured.value = null
  }
}

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
let rateTimer
onMounted(() => {
  startAutoReveal()
  checkHealth()
  loadFeatured()
  loadRates()
  healthTimer = setInterval(checkHealth, 30000)
  rateTimer = setInterval(loadRates, 5 * 60 * 1000)
})
onBeforeUnmount(() => {
  clearInterval(healthTimer)
  clearInterval(rateTimer)
})
</script>

<style scoped>
:deep(.v-toolbar__content) {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, auto) minmax(0, 1fr);
  align-items: center;
}

.bar-nav {
  display: flex;
  align-items: center;
  justify-content: safe center;
  gap: 4px;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.bar-nav::-webkit-scrollbar { display: none; }

.bar-link {
  display: inline-flex;
  align-items: center;
  flex: none;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 6px;
  color: #fff;
  font-size: 14px;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
  opacity: 0.85;
  transition: background-color 150ms ease, opacity 150ms ease;
}

.bar-link.is-here {
  opacity: 1;
  background: rgba(255, 255, 255, 0.18);
}

.bar-link:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 2px;
}

.bar-end {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  min-width: 0;
  gap: 4px;
}

.name-chip {
  max-width: 42vw;
}

.name-chip :deep(.v-chip__content) {
  overflow: hidden;
  text-overflow: ellipsis;
}

.health-dot {
  margin-left: 6px;
  vertical-align: middle;
}

.sol-bar.is-phone :deep(.v-toolbar__content) {
  grid-template-columns: auto minmax(0, 1fr);
}

.phone-nav {
  padding-bottom: env(safe-area-inset-bottom);
  height: calc(56px + env(safe-area-inset-bottom)) !important;
}

.phone-nav :deep(.v-btn) {
  flex: 1 1 0;
  min-width: 0 !important;
  padding-inline: 2px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.85) !important;
}

.phone-nav :deep(.v-btn--active),
.phone-nav :deep(.v-btn--selected) {
  color: #fff !important;
  background: rgba(255, 255, 255, 0.18);
}

.phone-nav :deep(.v-icon) {
  font-size: 24px;
}
</style>
