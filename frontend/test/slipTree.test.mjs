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

// The program test and the keeper test pin the same two hashes.
test('a canopy proof stays inside the coupon chunk', async () => {
  const { bottomProof, canopyChunk } = await import('../src/services/slipTree.js')
  const { base58Encode } = await import('../src/services/solana.js')
  const hex = bytes => [...bytes].map(byte => byte.toString(16).padStart(2, '0')).join('')
  const owner = base58Encode(new Uint8Array(32).fill(1))
  const leaves = new Map()
  for (let index = 1024; index < 1030; index += 1) leaves.set(index, slipLeaf(index, owner, 1, new Uint8Array(32).fill(index & 0xff)))
  const proof = bottomProof(leaves, 1027, 10)
  assert.equal(proof.length, 10)
  assert.equal(hex(slipFold(1027, leaves.get(1027), proof)), '6dc83f88ac175a33282aed66fd901da954f9fd9bd97b66c62a78d24bf78ab65d')
  assert.equal(hex(proof[1]), '8f181aec88bc669959be78fe4878bdf1a7b7cfec7eb7da2a16df616ca7f8b93d')

  // Coupons of other chunks do not change it, and an older event's whole proof starts with it.
  const all = Array.from({ length: 1030 }, () => null)
  for (const [index, leaf] of leaves) all[index] = leaf
  for (let index = 0; index < 1024; index += 97) {
    all[index] = slipLeaf(index, owner, 2, new Uint8Array(32).fill(9))
    leaves.set(index, all[index])
  }
  assert.deepEqual(bottomProof(leaves, 1027, 10), proof)
  assert.deepEqual(sparseProof(all, 1027).slice(0, 10), proof)
  assert.deepEqual(canopyChunk(1027, 10), { from: 1024, to: 2048 })
  assert.deepEqual(canopyChunk(999_999, 10), { from: 999_424, to: 1_000_448 })
})

test('coupon 1,000,000 fits in one transaction on an event with a canopy', async () => {
  globalThis.window = { addEventListener () {} }
  const program = await import('../src/services/program.js')
  const { bottomProof, slipRevealLeaf } = await import('../src/services/slipTree.js')
  const { base58Encode } = await import('../src/services/solana.js')
  const key = fill => base58Encode(new Uint8Array(32).fill(fill))
  const [owner, admin, blockhash] = [key(1), key(2), key(3)]
  const index = 999_999
  const nonce = 7
  const event = program.eventPda(admin, nonce)
  const commitment = new Uint8Array(32).fill(7)
  const picks = new Uint8Array(13).fill(0b111)
  const colors = program.colorsBytes(Array.from({ length: 4 }, () => ({ signs: '1', min: 0, max: 13 })))
  // A full last chunk with no empty hash to drop: every proof hash is a real node.
  const entries = new Map()
  const reveals = new Map()
  for (let at = 999_424; at <= index; at += 1) {
    entries.set(at, slipLeaf(at, owner, 14, commitment))
    reveals.set(at, slipRevealLeaf(at, owner, 14, commitment, picks, colors))
  }
  const entryProof = bottomProof(entries, index, 10)
  const revealProof = bottomProof(reveals, index, 10)
  const reveal = { index, owner, combinations: 14, picks, revealNonce: 5n, commitment, colors }
  const size = instructions => program.serializeTransaction(program.compileMessage(owner, instructions, blockhash)).length
  // A colored coupon also sends a compute limit.
  const budget = program.computeBudgetIx(400_000)

  const sizes = {
    place: size([program.placeCanopySlipIx({ player: owner, admin, nonce, commitment, combinations: 14 })]),
    reveal: size([budget, program.revealCanopySlipIx({ caller: owner, admin, nonce, ...reveal, entryProof, revealProof })]),
    score: size([budget, program.scoreCanopySlipIx({ caller: owner, admin, event, reveal, revealed: true, revealProof })]),
    claim: size([budget, program.claimCanopySlipIx({ owner, admin, nonce, reveal, revealProof })]),
    refund: size([program.refundCanopySlipIx({ owner, admin, nonce, index, combinations: 14, commitment, entryProof })])
  }
  for (const [name, bytes] of Object.entries(sizes)) assert.ok(bytes <= 1232, `${name} is ${bytes} bytes`)

  // The same coupon on an event without a canopy sends three whole proofs and does not fit.
  const whole = Array.from({ length: 20 }, (_, level) => new Uint8Array(32).fill(level + 1))
  assert.throws(() => size([program.scoreSlipIx({
    caller: owner, admin, event, reveal, revealed: true, entryProof: whole, revealProof: whole, scoredProof: whole
  })]), /too large/)
  // A proof of the wrong length is refused before it is signed.
  assert.throws(() => program.claimCanopySlipIx({ owner, admin, nonce, reveal, revealProof: revealProof.slice(0, 9) }), /10 hashes/)
})

test('a new event creates its canopy and reads it back', async () => {
  globalThis.window = { addEventListener () {} }
  const program = await import('../src/services/program.js')
  const { base58Encode } = await import('../src/services/solana.js')
  const admin = base58Encode(new Uint8Array(32).fill(2))
  const create = program.createEventIx({ admin, other: admin, nonce: 7, deadline: 10, revealDeadline: 20 })
  const event = program.eventPda(admin, 7)
  const canopy = create.accounts[create.accounts.length - 2]
  assert.equal(canopy.pubkey, program.canopyPda(event))
  assert.equal(canopy.isWritable, true)
  assert.equal(create.accounts.at(-1).pubkey, program.SYSTEM_PROGRAM)

  const reveal = program.revealCanopySlipIx({
    caller: admin, admin, nonce: 7, index: 3, owner: admin, combinations: 1, picks: new Uint8Array(13), revealNonce: 77n,
    commitment: new Uint8Array(32), colors: new Uint8Array(0),
    entryProof: Array.from({ length: 10 }, () => new Uint8Array(32)), revealProof: Array.from({ length: 10 }, () => new Uint8Array(32))
  })
  assert.equal(program.revealNonceFromInstruction(reveal.data), 77n)

  // The admin page parks a rollover and creates the next event in one transaction.
  const tiers = Array.from({ length: 8 }, (_, tier) => ({ correct: 13 - tier, percent: 10 }))
  const park = program.parkRolloverIx({ admin, source: base58Encode(new Uint8Array(32).fill(5)) })
  const wide = program.createEventIx({ admin, other: base58Encode(new Uint8Array(32).fill(4)), nonce: 7, deadline: 10, revealDeadline: 20, gameCount: 13, tiers })
  const sent = program.serializeTransaction(program.compileMessage(admin, [park, wide], base58Encode(new Uint8Array(32).fill(3))))
  assert.ok(sent.length <= 1232, `park and create is ${sent.length} bytes`)
})

test('the challenge bond is 10% of the player pot, clamped to the event limits', async () => {
  const { createEventIx, disputeBond } = await import('../src/services/program.js')
  const sol = 1_000_000_000n
  assert.equal(disputeBond({ sales: 100n * sol, tiers: [{ percent: 100 }], disputeMin: 10, disputeMax: 50 }), 10n * sol)
  assert.equal(disputeBond({ sales: 250n * sol, tiers: [{ percent: 100 }], disputeMin: 10, disputeMax: 50 }), 25n * sol)
  assert.equal(disputeBond({ sales: 500n * sol, tiers: [{ percent: 100 }], disputeMin: 10, disputeMax: 50 }), 50n * sol)
  assert.equal(disputeBond({ sales: 400_000_000n, tiers: [{ percent: 90 }] }), 10n * sol)
  const admin = '11111111111111111111111111111111'
  const created = createEventIx({ admin, other: admin, nonce: 7, deadline: 10, revealDeadline: 20 })
  assert.deepEqual([...created.data.slice(-2)], [10, 50])
  const custom = createEventIx({ admin, other: admin, nonce: 7, deadline: 10, revealDeadline: 20, disputeMin: 20, disputeMax: 40 })
  assert.deepEqual([...custom.data.slice(-2)], [20, 40])
  assert.throws(() => createEventIx({ admin, other: admin, nonce: 7, deadline: 10, revealDeadline: 20, disputeMin: 40, disputeMax: 10 }))
})
