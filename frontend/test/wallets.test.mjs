import assert from 'node:assert/strict'
import test from 'node:test'
import {
  SOLANA_CHAIN,
  canSendLegacy,
  offeredWallets,
  signatureParts,
  signInInput,
  transactionRequest,
  transactionSignature,
  walletIcon,
  walletKind,
  wantsOffchainSignIn
} from '../src/services/walletKinds.js'

test('only the wallets this phase ships are recognized', () => {
  assert.equal(walletKind('Phantom'), 'phantom')
  assert.equal(walletKind('Jupiter Wallet'), 'jupiter')
  assert.equal(walletKind('Jupiter'), 'jupiter')
  assert.equal(walletKind('Solflare'), 'solflare')
  assert.equal(walletKind('Backpack'), 'backpack')
  assert.equal(walletKind('Jupiter Mobile'), 'mobile')
  assert.equal(walletKind('Mobile Wallet Adapter'), 'mobile')
  assert.equal(walletKind('Solana Mobile Wallet Adapter'), 'mobile')
  assert.equal(walletKind('Jupiter ID'), null)
  assert.equal(walletKind('Trust Wallet'), null)
  assert.equal(walletKind('Phantom Flare'), null)
})

test('a phone offers Jupiter Mobile and a desktop does not', () => {
  assert.equal(offeredWallets(false).some(item => item.kind === 'mobile'), false)
  assert.equal(offeredWallets(true).some(item => item.kind === 'mobile'), true)
  assert.equal(offeredWallets(true).filter(item => item.kind === 'jupiter').length, 1)
})

test('sign-in asks for devnet and wraps for a ledger only when the wallet can', () => {
  const older = signInInput({
    host: 'localhost:3000',
    origin: 'http://localhost:3000',
    address: 'wallet',
    nonce: 'abc',
    issuedAt: '2026-11-12T20:00:00.000Z',
    version: '1.0.0'
  })
  assert.equal(older.chainId, SOLANA_CHAIN)
  assert.equal(older.domain, 'localhost:3000')
  assert.equal(older.useOffchainMessage, undefined)
  assert.equal(wantsOffchainSignIn('1.0.0'), false)
  assert.equal(wantsOffchainSignIn('1.0'), false)
  assert.equal(wantsOffchainSignIn(''), false)

  const ledger = signInInput({ ...older, version: '1.1.0', address: 'wallet' })
  assert.deepEqual(ledger.useOffchainMessage, { messageVersion: 1 })
  assert.equal(wantsOffchainSignIn('1.2.0'), true)
  assert.equal(signInInput({ ...older, version: '2.0.0' }).chainId, 'solana:devnet')
})

test('a payment is requested on devnet', () => {
  const bytes = new Uint8Array([1, 2, 3])
  const request = transactionRequest({ address: 'wallet' }, bytes)
  assert.equal(request.chain, 'solana:devnet')
  assert.equal(request.transaction, bytes)
  assert.equal(canSendLegacy(undefined), true)
  assert.equal(canSendLegacy(['legacy']), true)
  assert.equal(canSendLegacy([0]), true)
  assert.equal(canSendLegacy(['versioned']), false)
})

test('a transaction signature is 64 bytes', () => {
  const signature = new Uint8Array(64).fill(9)
  assert.equal(transactionSignature(signature), signature)
  assert.throws(() => transactionSignature(signature.slice(0, 63)), /unexpected signature/)
})

test('the mobile adapter signature is removed from the end of the signed message', () => {
  const message = new TextEncoder().encode('Sign in to Sol (devnet)')
  const signature = new Uint8Array(64).fill(4)
  const combined = new Uint8Array(message.length + signature.length)
  combined.set(message)
  combined.set(signature, message.length)
  const stripped = signatureParts({ signature, signedMessage: combined }, message)
  assert.deepEqual(stripped.signedMessage, message)
  assert.deepEqual(stripped.signature, signature)

  const raw = signatureParts({ signature, signedMessage: message }, message)
  assert.deepEqual(raw.signedMessage, message)
})

test('a wallet icon has to be an image', () => {
  assert.equal(walletIcon('https://example.com/icon.png'), 'https://example.com/icon.png')
  assert.equal(walletIcon('data:image/svg+xml;base64,PHN2Zw=='), 'data:image/svg+xml;base64,PHN2Zw==')
  assert.equal(walletIcon('javascript:alert(1)'), '')
  assert.equal(walletIcon('data:text/html,hi'), '')
})
