import { reactive } from 'vue'
import { getWallets } from '@wallet-standard/app'
import {
  SOLANA_CHAIN,
  canSendLegacy,
  offeredWallets,
  signatureParts,
  signInInput,
  transactionRequest,
  transactionSignature,
  walletIcon,
  walletKind
} from './walletKinds.js'

const NAME_KEY = 'walletName'

export const walletChoices = reactive({ list: [], open: false })

let active = null
let sessionAddress = null
let unsubscribe = () => {}
let onAccount = () => {}
let starting = null

export function rememberAccount (address) {
  sessionAddress = address || null
}

export function onWalletAccount (handler) {
  onAccount = handler
}

export function selectedAccount () {
  return active ? (firstAccount(active)?.address || null) : null
}

export function walletByName (name) {
  if (!name || typeof window === 'undefined') return null
  return getWallets().get().find(wallet => wallet.name === name) || null
}

export function openLogin () {
  refresh()
  walletChoices.open = true
}

export function choiceLabel (item) {
  return item.kind === 'mobile' ? 'Jupiter Mobile' : item.name
}

export function missingWallets (phone) {
  const installed = new Set(walletChoices.list.map(item => item.kind))
  return offeredWallets(phone).filter(item => !installed.has(item.kind))
}

function firstAccount (wallet) {
  return wallet?.accounts?.find(account => account?.address) || null
}

function refresh () {
  if (typeof window === 'undefined') return
  const seen = new Set()
  const list = []
  for (const wallet of getWallets().get()) {
    const kind = walletKind(wallet.name)
    if (!kind || seen.has(wallet.name)) continue
    seen.add(wallet.name)
    list.push({ kind, name: wallet.name, icon: walletIcon(wallet.icon) })
  }
  const order = new Map(offeredWallets(true).map((item, index) => [item.kind, index]))
  list.sort((a, b) => (order.get(a.kind) ?? 99) - (order.get(b.kind) ?? 99) || a.name.localeCompare(b.name))
  walletChoices.list = list
}

function rememberedName () {
  try {
    return localStorage.getItem(NAME_KEY) || ''
  } catch {
    return ''
  }
}

function rememberName (name) {
  try {
    if (name) localStorage.setItem(NAME_KEY, name)
    else localStorage.removeItem(NAME_KEY)
  } catch {
    // Private mode can refuse storage. The session cookie still signs the user in.
  }
}

function setActive (wallet) {
  if (active !== wallet) {
    unsubscribe()
    unsubscribe = () => {}
    active = wallet
    const on = wallet.features?.['standard:events']?.on
    if (typeof on === 'function') {
      const stop = on('change', () => onAccount(firstAccount(wallet)?.address || null))
      if (typeof stop === 'function') unsubscribe = stop
    }
  }
  rememberName(wallet.name)
}

export async function connectPrompt (wallet) {
  const connect = wallet?.features?.['standard:connect']?.connect
  if (typeof connect !== 'function') throw new Error('This wallet cannot connect')
  let account = firstAccount(wallet)
  if (!account) {
    const result = await connect()
    account = result?.accounts?.[0] || firstAccount(wallet)
  }
  if (!account?.address) throw new Error('The wallet did not share an account')
  setActive(wallet)
  return account
}

function sameSession (account) {
  if (!account?.address) throw new Error('The wallet did not share an account')
  if (sessionAddress && account.address !== sessionAddress) {
    throw new Error('The connected account is not the one that signed in')
  }
  return account
}

async function accountForSigning () {
  if (!active) {
    const wallet = walletByName(rememberedName())
    if (!wallet) throw new Error('Connect your wallet')
    return sameSession(await connectPrompt(wallet))
  }
  return sameSession(firstAccount(active) || await connectPrompt(active))
}

function bytesToBase64 (bytes) {
  let binary = ''
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  return btoa(binary)
}

function signInUnavailable (error) {
  return /no sign in result|does not support sign in|signIn is not supported|method not found/i.test(String(error?.message || error))
}

export async function signLogin (wallet, account, { nonce, message }) {
  const address = account?.address
  if (!address) throw new Error('The wallet did not share an account')
  const signIn = wallet.features?.['solana:signIn']
  if (typeof signIn?.signIn === 'function') {
    try {
      const [result] = await signIn.signIn(signInInput({
        host: location.host,
        origin: location.origin,
        address,
        nonce,
        issuedAt: new Date().toISOString(),
        version: signIn.version
      }))
      const signedAs = result?.account?.address || address
      if (signedAs !== address) throw new Error('The wallet signed in with a different account')
      const parts = signatureParts(result)
      return {
        publicKey: address,
        signature: bytesToBase64(parts.signature),
        signedMessage: bytesToBase64(parts.signedMessage)
      }
    } catch (error) {
      if (!signInUnavailable(error)) throw error
    }
  }
  const feature = wallet.features?.['solana:signMessage']
  if (typeof feature?.signMessage !== 'function') throw new Error('This wallet cannot sign in')
  const requested = new TextEncoder().encode(message)
  const [result] = await feature.signMessage({ account, message: requested })
  const parts = signatureParts(result, requested)
  return {
    publicKey: address,
    signature: bytesToBase64(parts.signature),
    signedMessage: bytesToBase64(parts.signedMessage)
  }
}

export async function signWithWallet (message) {
  const account = await accountForSigning()
  const feature = active.features?.['solana:signMessage']
  if (typeof feature?.signMessage !== 'function') throw new Error('This wallet cannot sign a message')
  const [result] = await feature.signMessage({ account, message })
  return signatureParts(result, message).signature
}

export async function signTransactionWithWallet (serialized) {
  const account = await accountForSigning()
  const feature = active.features?.['solana:signTransaction']
  if (typeof feature?.signTransaction !== 'function') throw new Error('This wallet cannot sign a transaction that needs the reveal keeper')
  if (!canSendLegacy(feature.supportedTransactionVersions)) throw new Error('This wallet cannot sign this transaction')
  const [result] = await feature.signTransaction(transactionRequest(account, serialized))
  const signed = transactionBytes(result?.signedTransaction)
  if (signed && signed.length > 64) return signed
  const signature = transactionBytes(result?.signature)
  if (signature?.length !== 64) throw new Error('The wallet did not return the signed transaction')
  const out = new Uint8Array(serialized)
  out.set(signature, 1)
  return out
}

function transactionBytes (value) {
  if (value instanceof Uint8Array) return value
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
  if (Array.isArray(value)) return Uint8Array.from(value)
  if (typeof value === 'string' && value) {
    const binary = atob(value)
    const out = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i)
    return out
  }
  return null
}

export async function sendWithWallet (serialized) {
  const account = await accountForSigning()
  const feature = active.features?.['solana:signAndSendTransaction']
  if (typeof feature?.signAndSendTransaction !== 'function') throw new Error('This wallet cannot send a transaction')
  if (!canSendLegacy(feature.supportedTransactionVersions)) throw new Error('This wallet cannot send this transaction')
  const [result] = await feature.signAndSendTransaction(transactionRequest(account, serialized))
  return transactionSignature(result?.signature)
}

export async function disconnectWallet () {
  const wallet = active
  unsubscribe()
  unsubscribe = () => {}
  active = null
  rememberName('')
  try {
    await wallet?.features?.['standard:disconnect']?.disconnect?.()
  } catch {
    // The wallet was already disconnected.
  }
}

export async function reattach () {
  const wallet = walletByName(rememberedName())
  const connect = wallet?.features?.['standard:connect']?.connect
  if (typeof connect !== 'function') return
  try {
    await connect({ silent: true })
  } catch {
    return
  }
  if (firstAccount(wallet)) setActive(wallet)
}

// Android Chrome can hand the page to Jupiter Mobile through the Solana Mobile Wallet Adapter.
// A desktop QR is not registered: production wallets do not complete that remote pairing.
async function registerPhoneWallet () {
  if (!/Android/i.test(navigator.userAgent)) return
  try {
    const { registerMwa } = await import('@solana-mobile/wallet-standard-mobile')
    registerMwa({
      appIdentity: { name: 'Edge', uri: location.origin, icon: '/favicon.svg' },
      chains: [SOLANA_CHAIN]
    })
  } catch {
    // The phone can still open this page inside the wallet's own browser.
  }
}

export function startWallets () {
  if (starting) return starting
  starting = (async () => {
    if (typeof window === 'undefined') return
    const api = getWallets()
    api.on('register', refresh)
    api.on('unregister', refresh)
    refresh()
    window.addEventListener('load', refresh, { once: true })
    // An in-app browser can inject its wallet after this script, sometimes without a register event.
    const timer = setInterval(refresh, 250)
    setTimeout(() => clearInterval(timer), 10000)
    await registerPhoneWallet()
    refresh()
  })()
  return starting
}
