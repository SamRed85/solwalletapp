import { reactive } from 'vue'
import { authApi, configApi, setUnauthorizedHandler } from '@/services/api'
import { SESSION_KEY, clearSession, loadSession, saveSession } from '@/services/session'
import { connectWallet, disconnectWallet, getBalance, getProvider, onPhantomAccount, signMessage, trustedAccount } from '@/services/solana'
import { notify } from '@/stores/notify'

export const auth = reactive({
  wallet: null,
  displayName: '',
  isAdmin: false,
  balance: null,
  busy: false,
  config: null
})

function apply (session) {
  auth.wallet = session?.wallet ?? null
  auth.displayName = session?.displayName ?? ''
  auth.isAdmin = session?.isAdmin ?? false
  if (!session) auth.balance = null
}

export function setProfile (user) {
  const session = loadSession()
  if (!session) return
  session.displayName = user.displayName
  session.isAdmin = user.isAdmin
  saveSession(session)
  apply(session)
}

export async function loadConfig () {
  auth.config ??= await configApi.get()
  return auth.config
}

export async function refreshBalance () {
  if (!auth.wallet) return
  try {
    await loadConfig()
    auth.balance = await getBalance(auth.config.solanaRpcUrl, auth.wallet)
  } catch {
    auth.balance = null
  }
}

export async function login () {
  auth.busy = true
  try {
    const wallet = await connectWallet()
    const { nonce, message } = await authApi.nonce(wallet)
    const signature = await signMessage(message)
    const { expiresAt, user } = await authApi.login(wallet, nonce, signature)
    const session = { expiresAt, wallet: user.walletAddress, displayName: user.displayName || '', isAdmin: user.isAdmin }
    saveSession(session)
    apply(session)
    refreshBalance()
  } finally {
    auth.busy = false
  }
}

export async function logout () {
  try {
    await authApi.logout()
  } catch {
    // The cookie is cleared locally even when the server is unreachable.
  }
  clearSession()
  apply(null)
  await disconnectWallet()
}

async function restore () {
  const session = loadSession()
  apply(session)
  if (!session) return
  try {
    const me = await authApi.me()
    session.displayName = me.displayName || ''
    session.isAdmin = me.isAdmin
    saveSession(session)
    apply(session)
    refreshBalance()
  } catch {
    // 401 is handled by the unauthorized handler; other errors keep the cached session
  }
}

setUnauthorizedHandler(() => {
  clearSession()
  apply(null)
})

window.addEventListener('storage', event => {
  if (event.key === SESSION_KEY) apply(loadSession())
})

let leaving = false

// The session belongs to one wallet. A different Phantom account, or a disconnect, ends it.
async function leaveIfWalletChanged (next) {
  if (leaving || !auth.wallet || next === auth.wallet) return
  leaving = true
  try {
    await logout()
    notify('Signed out because the Phantom account changed', 'info')
  } finally {
    leaving = false
  }
}

async function syncPhantomAccount () {
  if (!auth.wallet) return
  const next = await trustedAccount()
  if (next) await leaveIfWalletChanged(next)
}

onPhantomAccount(next => { void leaveIfWalletChanged(next) })

window.addEventListener('focus', () => { void syncPhantomAccount() })
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') void syncPhantomAccount()
})

setInterval(() => {
  const next = getProvider()?.publicKey?.toBase58?.()
  if (next) void leaveIfWalletChanged(next)
}, 1000)

export const authReady = restore().then(syncPhantomAccount)
