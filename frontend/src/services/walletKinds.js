export const SOLANA_CHAIN = 'solana:devnet'

export const WALLETS = [
  { kind: 'jupiter', label: 'Jupiter Wallet', href: 'https://jup.ag/wallet' },
  { kind: 'phantom', label: 'Phantom', href: 'https://phantom.app/' },
  { kind: 'solflare', label: 'Solflare', href: 'https://solflare.com/' },
  { kind: 'backpack', label: 'Backpack', href: 'https://backpack.app/' },
  { kind: 'mobile', label: 'Jupiter Mobile', href: 'https://jup.ag/wallet' }
]

const SIGN_IN_STATEMENT = 'Sign in to Sol (devnet). This is free and does not send a transaction.'

export function walletKind (name) {
  const normalized = String(name || '').trim().toLowerCase()
  if (normalized === 'phantom') return 'phantom'
  if (normalized === 'solflare') return 'solflare'
  if (normalized === 'backpack') return 'backpack'
  if (normalized === 'jupiter' || normalized === 'jupiter wallet') return 'jupiter'
  if (normalized === 'jupiter mobile' || normalized === 'mobile wallet adapter' || normalized === 'solana mobile wallet adapter') return 'mobile'
  return null
}

export function offeredWallets (phone) {
  return WALLETS.filter(item => item.kind !== 'mobile' || phone)
}

const ICON = /^(?:data:image\/(?:png|jpeg|gif|webp|svg\+xml)(?:;base64)?,|https:\/\/)/

export function walletIcon (icon) {
  return typeof icon === 'string' && icon.length <= 200000 && ICON.test(icon) ? icon : ''
}

// Version 1.1.0 of solana:signIn is the first that understands useOffchainMessage.
export function wantsOffchainSignIn (version) {
  const [major, minor] = String(version || '').split('.').map(Number)
  if (!Number.isInteger(major) || !Number.isInteger(minor)) return false
  return major > 1 || (major === 1 && minor >= 1)
}

export function signInInput ({ host, origin, address, nonce, issuedAt, version }) {
  const input = {
    domain: host,
    address,
    statement: SIGN_IN_STATEMENT,
    uri: origin,
    version: '1',
    chainId: SOLANA_CHAIN,
    nonce,
    issuedAt
  }
  if (wantsOffchainSignIn(version)) input.useOffchainMessage = { messageVersion: 1 }
  return input
}

export function canSendLegacy (versions) {
  if (!versions) return true
  return versions.includes('legacy') || versions.includes(0)
}

export function transactionRequest (account, transaction) {
  return { account, transaction, chain: SOLANA_CHAIN }
}

export function transactionSignature (signature) {
  const bytes = toBytes(signature)
  if (bytes.length !== 64) throw new Error('The wallet returned an unexpected signature')
  return bytes
}

function toBytes (value) {
  if (value instanceof Uint8Array) return value
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
  if (Array.isArray(value)) return Uint8Array.from(value)
  return new Uint8Array()
}

function sameBytes (left, right) {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i++) if (left[i] !== right[i]) return false
  return true
}

// The Mobile Wallet Adapter appends the 64-byte signature to the bytes it calls the signed message.
export function signatureParts (result, requested) {
  const signature = toBytes(result?.signature)
  if (signature.length !== 64) throw new Error('The wallet returned an unexpected signature')
  let signed = toBytes(result?.signedMessage)
  if (!signed.length) signed = toBytes(requested)
  if (signed.length > 64 && sameBytes(signed.subarray(signed.length - 64), signature)) {
    signed = signed.subarray(0, signed.length - 64)
  }
  if (!signed.length) throw new Error('The wallet did not return the signed message')
  return { signature, signedMessage: signed }
}
