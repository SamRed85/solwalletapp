<template>
  <v-app :class="{ 'glow-band': route.name === 'Leaderboard' }">
    <v-app-bar class="sol-bar" :class="{ 'is-phone': mobile }" color="transparent" elevation="0" :height="mobile ? 56 : 64">
      <v-app-bar-title>
        <button v-if="couponOpen" type="button" class="bar-back" aria-label="Back to my coupons" @click="closeCoupon">
          <v-icon icon="mdi-arrow-left" size="22" />
          Back
        </button>
        <span v-else class="wordmark">Edge</span>
        <v-icon
          v-if="mobile && !couponOpen" class="health-dot" size="8"
          :color="healthy === false ? 'error' : 'success'"
          :title="healthy === null ? 'Checking the backend' : healthy ? 'Backend healthy' : 'Backend unreachable'"
        >mdi-circle</v-icon>
      </v-app-bar-title>

      <nav v-show="!mobile" class="bar-nav" aria-label="Pages">
        <template v-for="item in menuItems" :key="item.to">
          <router-link
            v-if="!item.auth || auth.wallet"
            :to="item.to"
            class="bar-link"
            active-class=""
            exact-active-class="is-here"
          >
            <v-icon :icon="item.icon" size="small" />
            {{ item.title }}
          </router-link>
          <button v-else type="button" class="bar-link" @click="onLogin">
            <v-icon :icon="item.icon" size="small" />
            {{ item.title }}
          </button>
        </template>
      </nav>

      <div class="bar-end">
        <v-btn
          v-if="mobile" icon variant="text" size="small"
          :aria-label="isDark ? 'Switch to light' : 'Switch to dark'"
          @click="toggleTheme"
        >
          <v-icon>{{ isDark ? 'mdi-weather-night' : 'mdi-weather-sunny' }}</v-icon>
        </v-btn>
        <template v-if="auth.wallet">
          <v-chip
            to="/profile"
            class="name-chip" :class="mobile ? 'mr-1' : 'mr-2'" :size="mobile ? 'x-small' : 'small'" variant="flat" :color="auth.isAdmin ? 'warning' : 'success'"
            :prepend-icon="auth.isAdmin ? 'mdi-shield-crown' : 'mdi-account'"
            title="Profile"
          >
            {{ auth.displayName || shortAddress(auth.wallet) }}
          </v-chip>
          <v-btn v-if="mobile" icon="mdi-logout" size="small" aria-label="Logout" @click="onLogout" />
          <v-btn v-else prepend-icon="mdi-logout" @click="onLogout">Logout</v-btn>
        </template>
        <v-btn v-else variant="outlined" prepend-icon="mdi-wallet" :loading="auth.busy" @click="onLogin">
          Connect
        </v-btn>
      </div>
    </v-app-bar>

    <v-main :class="{ 'has-phone-nav': mobile }">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.path" />
        </transition>
      </router-view>
    </v-main>

    <v-bottom-navigation v-if="mobile" :model-value="navValue" class="phone-nav" grow :height="60">
      <v-btn v-for="item in phoneItems" :key="item.to" :value="item.to" :to="item.auth && !auth.wallet ? undefined : item.to" @click="guard(item)">
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
import { authApi, healthApi } from '@/services/api'
import { startAutoReveal } from '@/services/autoReveal'
import { auth, login, logout, setProfile } from '@/stores/auth'
import { loadRates } from '@/stores/currency'
import { notify, notifyError, snackbar } from '@/stores/notify'
import { shortAddress } from '@/utils/format'

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

const phoneItems = [
  { title: 'Home', icon: 'mdi-home', to: '/' },
  { title: 'Coupons', icon: 'mdi-ticket', to: '/coupons', auth: true },
  { title: 'Leaderboard', icon: 'mdi-trophy', to: '/leaderboard' },
  { title: 'Profile', icon: 'mdi-account', to: '/profile', auth: true }
]

const menuItems = computed(() => [
  ...phoneItems,
  ...(auth.isAdmin
    ? [
        { title: 'Admin', icon: 'mdi-shield-crown', to: '/admin' },
        { title: 'Users', icon: 'mdi-account-group', to: '/users' }
      ]
    : [])
])

const navValue = computed(() => phoneItems.find(item => item.to === route.path)?.to ?? route.path)

const couponOpen = computed(() => route.name === 'MyCoupons' && !!route.query.coupon)

function closeCoupon () {
  if (window.history.state?.back) router.back()
  else router.replace({ path: '/coupons' })
}

function guard (item) {
  if (item.auth && !auth.wallet) onLogin()
}

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
let rateTimer
onMounted(() => {
  startAutoReveal()
  checkHealth()
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

button.bar-link {
  background: none;
  border: 0;
  cursor: pointer;
}

.bar-link {
  display: inline-flex;
  align-items: center;
  flex: none;
  gap: 6px;
  padding: 6px 12px;
  border-radius: var(--radius-pill);
  color: rgb(var(--v-theme-on-surface));
  font-size: var(--fs-small);
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
  opacity: 0.62;
  transition: background-color 150ms ease, opacity 150ms ease;
}

.bar-link.is-here {
  opacity: 1;
  background: rgb(var(--v-theme-surface-variant));
}

.bar-link:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
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

.sol-bar.is-phone .name-chip {
  max-width: 30vw;
}

.balance-chip {
  flex: none;
}

.name-chip :deep(.v-chip__content) {
  overflow: hidden;
  text-overflow: ellipsis;
}

.bar-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px 6px 4px;
  border: 0;
  border-radius: var(--radius-pill);
  background: none;
  color: rgb(var(--v-theme-on-surface));
  font: inherit;
  font-size: var(--fs-body);
  font-weight: 600;
  cursor: pointer;
}

.bar-back:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: 2px;
}

.health-dot {
  margin-left: 6px;
  vertical-align: middle;
}

.sol-bar.is-phone :deep(.v-toolbar__content) {
  grid-template-columns: auto minmax(0, 1fr);
}

.sol-bar {
  background-color: rgb(var(--v-theme-background)) !important;
  background-image: var(--page-glow) !important;
  background-attachment: fixed !important;
  border-bottom: none;
}

.phone-nav {
  position: fixed;
  left: 16px !important;
  right: 16px !important;
  bottom: calc(16px + var(--inset-bottom)) !important;
  width: auto !important;
  height: 60px !important;
  z-index: 8;
  border-radius: var(--radius-pill) !important;
  background: rgb(var(--v-theme-surface-variant)) !important;
  border: none;
  overflow: hidden;
}

.phone-nav :deep(.v-btn) {
  flex: 1 1 0;
  min-width: 0 !important;
  padding-inline: 2px;
  font-size: var(--fs-label);
  color: rgba(var(--v-theme-on-surface), 0.62) !important;
}

.phone-nav :deep(.v-btn--active),
.phone-nav :deep(.v-btn--selected) {
  color: rgb(var(--v-theme-primary)) !important;
  background: transparent !important;
}

.phone-nav :deep(.v-btn--active .v-btn__overlay),
.phone-nav :deep(.v-btn--selected .v-btn__overlay) {
  opacity: 0 !important;
}

.phone-nav :deep(.v-icon) {
  font-size: 22px;
  transition: color 180ms ease, transform 180ms ease;
}

.phone-nav :deep(.v-btn--active .v-icon),
.phone-nav :deep(.v-btn--selected .v-icon) {
  animation: nav-pop 320ms ease;
}

@keyframes nav-pop {
  0% { transform: scale(0.8); }
  55% { transform: scale(1.16); }
  100% { transform: scale(1); }
}

@media (prefers-reduced-motion: reduce) {
  .phone-nav :deep(.v-icon) { animation: none !important; }
}
</style>
