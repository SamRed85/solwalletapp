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
