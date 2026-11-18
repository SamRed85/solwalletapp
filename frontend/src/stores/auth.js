import { reactive } from 'vue'
import { authApi, configApi, setUnauthorizedHandler } from '@/services/api'
import { SESSION_KEY, clearSession, loadSession, saveSession } from '@/services/session'
import { getBalance } from '@/services/solana'
import {
  connectPrompt,
  disconnectWallet,
  onWalletAccount,
  openLogin,
  reattach,
  rememberAccount,
  selectedAccount,
  signLogin,
  startWallets,
  walletChoices
} from '@/services/wallets'
import { notify } from '@/stores/notify'

export { openLogin, walletChoices }

export const auth = reactive({
  wallet: null,
  displayName: '',
  isAdmin: false,
  balance: null,
  busy: false,
  config: null
})

// A request sent before this moment carried no cookie for the current session, so its 401
// says nothing about that session. A wallet's in-app browser also reports an empty account
// for a few seconds after the first approval. That report is not a disconnect.
let signedInAt = 0
const SIGN_IN_GRACE_MS = 5000

function apply (session) {
  auth.wallet = session?.wallet ?? null
  auth.displayName = session?.displayName ?? ''
  auth.isAdmin = session?.isAdmin ?? false
  if (!session) auth.balance = null
  rememberAccount(auth.wallet)
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

export async function login (wallet) {
  auth.busy = true
  try {
    const account = await connectPrompt(wallet)
    const { nonce, message } = await authApi.nonce(account.address)
    const signed = await signLogin(wallet, account, { nonce, message })
    const { expiresAt, user } = await authApi.login(signed.publicKey, nonce, signed.signature, signed.signedMessage)
    const session = { expiresAt, wallet: user.walletAddress, displayName: user.displayName || '', isAdmin: user.isAdmin }
    signedInAt = Date.now()
    saveSession(session)
    apply(session)
    walletChoices.open = false
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

setUnauthorizedHandler(sentAt => {
  if (sentAt < signedInAt) return
  clearSession()
  apply(null)
})

window.addEventListener('storage', event => {
  if (event.key === SESSION_KEY) apply(loadSession())
})

let leaving = false

// The session belongs to one wallet. An empty account does not end it: the in-app browser
// clears the account while the connection made a moment ago is still the approved one.
// A real switch arrives as a different address.
async function leaveIfWalletChanged (next) {
  if (leaving || !auth.wallet || !next || next === auth.wallet) return
  if (Date.now() - signedInAt < SIGN_IN_GRACE_MS) return
  leaving = true
  try {
    await logout()
    notify('Signed out because the wallet account changed', 'info')
  } finally {
    leaving = false
  }
}

function syncAccount () {
  if (!auth.wallet) return
  const next = selectedAccount()
  if (next) void leaveIfWalletChanged(next)
}

onWalletAccount(next => { void leaveIfWalletChanged(next) })

window.addEventListener('focus', () => { void syncAccount() })
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') void syncAccount()
})

setInterval(() => {
  const next = selectedAccount()
  if (next) void leaveIfWalletChanged(next)
}, 1000)

export const authReady = startWallets().then(restore).then(reattach).then(syncAccount)
