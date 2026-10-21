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

export async function getBalance (rpcUrl, publicKey) {
  const response = await fetch(rpcUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'getBalance', params: [publicKey, { commitment: 'confirmed' }] })
  })
  const { result, error } = await response.json()
  if (error) throw new Error(error.message)
  return result.value / 1e9
}
