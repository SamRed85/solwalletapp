export function getProvider () {
  const phantom = window.phantom?.solana
  if (phantom?.isPhantom) return phantom
  return window.solana?.isPhantom ? window.solana : null
}

export const isPhantomInstalled = () => getProvider() !== null

function requireProvider () {
  const provider = getProvider()
  if (!provider) throw new Error('Phantom wallet is not installed')
  return provider
}

export async function connectWallet () {
  const { publicKey } = await requireProvider().connect()
  return publicKey.toBase58()
}

// The account Phantom has selected, without opening an approval popup.
// Undefined means Phantom is not available. Null means it is not connected.
export async function trustedAccount () {
  const provider = getProvider()
  if (!provider) return undefined
  try {
    const { publicKey } = await provider.connect({ onlyIfTrusted: true })
    return publicKey?.toBase58?.() ?? null
  } catch {
    return provider.publicKey?.toBase58?.() ?? null
  }
}

export function onPhantomAccount (handler) {
  const bound = new Set()
  const bind = () => {
    const providers = [window.phantom?.solana, window.solana].filter(provider => provider?.isPhantom && !bound.has(provider))
    for (const provider of providers) {
      bound.add(provider)
      provider.on?.('accountChanged', key => {
        if (key?.toBase58) handler(key.toBase58())
        else if (typeof key === 'string' && key) handler(key)
        else handler(null)
      })
      provider.on?.('disconnect', () => handler(null))
    }
  }
  bind()
  window.addEventListener('load', bind, { once: true })
  const timer = setInterval(() => {
    bind()
    if (bound.size > 0) clearInterval(timer)
  }, 250)
  setTimeout(() => clearInterval(timer), 10000)
}

export async function disconnectWallet () {
  try {
    await getProvider()?.disconnect()
  } catch {
    // already disconnected
  }
}

export async function signMessage (message) {
  const { signature } = await requireProvider().signMessage(new TextEncoder().encode(message), 'utf8')
  return btoa(String.fromCharCode(...signature))
}

async function rpc (rpcUrl, method, params) {
  const response = await fetch(rpcUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
  })
  if (response.status === 429) throw new Error('Solana RPC rate limit reached, try again in a moment')
  const { result, error } = await response.json()
  if (error) throw new Error(error.message)
  return result
}

export async function getBalance (rpcUrl, publicKey) {
  const result = await rpc(rpcUrl, 'getBalance', [publicKey, { commitment: 'confirmed' }])
  return result.value / 1e9
}

const ALPHABET = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'

export function base58Encode (bytes) {
  let n = 0n
  for (const b of bytes) n = n * 256n + BigInt(b)
  let out = ''
  while (n > 0n) {
    out = ALPHABET[Number(n % 58n)] + out
    n /= 58n
  }
  for (const b of bytes) {
    if (b !== 0) break
    out = '1' + out
  }
  return out
}

export function base58Decode (text, length) {
  let n = 0n
  for (const c of text) {
    const v = ALPHABET.indexOf(c)
    if (v < 0) throw new Error('Invalid base58 string')
    n = n * 58n + BigInt(v)
  }
  const bytes = []
  while (n > 0n) {
    bytes.unshift(Number(n % 256n))
    n /= 256n
  }
  for (const c of text) {
    if (c !== '1') break
    bytes.unshift(0)
  }
  if (length && bytes.length !== length) throw new Error('Invalid Solana address')
  return Uint8Array.from(bytes)
}

const compactU16 = n => {
  const out = []
  for (;;) {
    const byte = n & 0x7f
    n >>= 7
    if (n === 0) return [...out, byte]
    out.push(byte | 0x80)
  }
}

// Legacy transaction message with one System Program transfer:
// header, account keys [payer (signer, writable), recipient (writable), system program], blockhash, instruction.
export function transferMessage (from, to, lamports, blockhash) {
  if (from === to) throw new Error('Sender and recipient must differ')
  const data = new Uint8Array(12)
  const view = new DataView(data.buffer)
  view.setUint32(0, 2, true)
  view.setBigUint64(4, BigInt(lamports), true)
  return Uint8Array.from([
    1, 0, 1,
    ...compactU16(3), ...base58Decode(from, 32), ...base58Decode(to, 32), ...new Uint8Array(32),
    ...base58Decode(blockhash, 32),
    ...compactU16(1), 2, ...compactU16(2), 0, 1, ...compactU16(data.length), ...data
  ])
}

// Phantom deserializes the payload as a complete transaction: a signature count, one
// 64-byte signature slot, then the message. Sending the message alone makes it treat
// part of the recipient address as the message version.
function unsignedTransaction (message) {
  return Uint8Array.from([1, ...new Uint8Array(64), ...message])
}

// Phantom submits the transaction to the network it is set to. The blockhash comes from devnet,
// so a wallet left on mainnet rejects the transaction instead of spending real SOL.
export async function sendPayment (rpcUrl, from, to, lamports) {
  const { value } = await rpc(rpcUrl, 'getLatestBlockhash', [{ commitment: 'confirmed' }])
  const message = transferMessage(from, to, lamports, value.blockhash)
  const { signature } = await requireProvider().request({
    method: 'signAndSendTransaction',
    params: { message: base58Encode(unsignedTransaction(message)) }
  })
  return { signature, lastValidBlockHeight: value.lastValidBlockHeight }
}

export async function waitForConfirmation (rpcUrl, signature, lastValidBlockHeight) {
  for (;;) {
    const { value } = await rpc(rpcUrl, 'getSignatureStatuses', [[signature]])
    const status = value[0]
    if (status?.err) throw new Error('The payment failed on chain')
    if (status?.confirmationStatus === 'confirmed' || status?.confirmationStatus === 'finalized') return
    if (lastValidBlockHeight && await rpc(rpcUrl, 'getBlockHeight', [{ commitment: 'confirmed' }]) > lastValidBlockHeight) {
      const { value: [final] } = await rpc(rpcUrl, 'getSignatureStatuses', [[signature], { searchTransactionHistory: true }])
      if (!final) throw new Error('The payment expired before it was confirmed. No SOL was sent.')
      if (final.err) throw new Error('The payment failed on chain')
      return
    }
    await new Promise(resolve => setTimeout(resolve, 1500))
  }
}
