import assert from 'node:assert/strict'
import test from 'node:test'
import { decodeBase58, proofFor, slipFold, slipLeaf, slipMark, slipZero, sparseProof } from '../src/services/slipTree.js'

test('a proof rebuilt from the archive matches the slip root', () => {
  assert.deepEqual(decodeBase58('11111111111111111111111111111111'), new Uint8Array(32))
  const owner = '11111111111111111111111111111111'
  const first = slipLeaf(0, owner, 1, new Uint8Array(32))
  const second = slipLeaf(1, owner, 1, new Uint8Array(32).fill(2))
  const empty = slipZero(0)

  const only = proofFor([first], 0)
  assert.deepEqual(only[0], empty)
  assert.deepEqual(slipFold(0, first, only), slipFold(0, first, Array.from({ length: 20 }, (_, level) => slipZero(level))))

  const proof = proofFor([first, second], 1)
  assert.deepEqual(proof[0], first)
  const root = slipFold(1, second, proof)
  assert.deepEqual(slipFold(0, first, proofFor([first, second], 0)), root)
  assert.notDeepEqual(root, slipFold(1, second, only))

  const marked = [slipMark(), null]
  const claimProof = sparseProof(marked, 1)
  assert.deepEqual(claimProof[0], slipMark())
  assert.deepEqual(sparseProof([first], 1)[0], first)
})

test('counting eight coupons fits in one Solana transaction', async () => {
  globalThis.window = { addEventListener () {} }
  const { compileMessage, scoreSlipIx, serializeTransaction } = await import('../src/services/program.js')
  const owner = '11111111111111111111111111111111'
  const commitment = new Uint8Array(32)
  const leaves = Array.from({ length: 8 }, (_, index) => slipLeaf(index, owner, 1, commitment))
  const reveal = {
    index: 7, owner, combinations: 1, picks: new Uint8Array(13), revealNonce: 0n, commitment
  }
  const proof = sparseProof(leaves, 7)
  const instruction = scoreSlipIx({
    caller: owner, admin: owner, event: owner, reveal, revealed: true,
    entryProof: proof, revealProof: proof, scoredProof: proof
  })
  const compiled = compileMessage(owner, [instruction], owner)
  const transaction = serializeTransaction(compiled)
  assert.ok(transaction.length <= 1232, `score transaction is ${transaction.length} bytes`)
})

test('a colored slip hashes like the program and the keeper', async () => {
  const { colorsBytes, couponCommitment, slipCommitment } = await import('../src/services/program.js')
  const { base58Encode } = await import('../src/services/solana.js')
  const { slipRevealLeaf } = await import('../src/services/slipTree.js')
  const hex = bytes => [...bytes].map(byte => byte.toString(16).padStart(2, '0')).join('')
  const owner = base58Encode(new Uint8Array(32).fill(1))
  const picks = new Uint8Array(13)
  picks.set([7, 7, 7])
  const colors = colorsBytes([{ signs: '73', min: 1, max: 2 }])
  assert.equal(hex(colors), '49000000000000000102')
  const commitment = slipCommitment(owner, picks, 7n, colors)
  assert.equal(hex(commitment), 'a202294cf5f800ffa813f4fa136a55f66013ad1427c533b33be77e3effdb6553')
  assert.equal(hex(slipRevealLeaf(3, owner, 18, commitment, picks, colors)), 'c27ac67dfad8d1e2c95fb654740478450fc5196fe39b1165be61ff3f853dc74c')
  assert.deepEqual(slipCommitment(owner, picks, 7n), couponCommitment(owner, picks, 7n))
})
