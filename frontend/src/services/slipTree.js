import { sha256 } from '@noble/hashes/sha2.js'

export const SLIP_DEPTH = 20

const BASE58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'

function hashv(parts) {
  let size = 0
  for (const part of parts) size += part.length
  const bytes = new Uint8Array(size)
  let offset = 0
  for (const part of parts) {
    bytes.set(part, offset)
    offset += part.length
  }
  return sha256(bytes)
}

function u32le(value) {
  const bytes = new Uint8Array(4)
  new DataView(bytes.buffer).setUint32(0, value, true)
  return bytes
}

export function decodeBase58 (text) {
  let zeroes = 0
  while (text[zeroes] === '1') zeroes += 1
  const bytes = []
  for (const char of text.slice(zeroes)) {
    let carry = BASE58.indexOf(char)
    if (carry < 0) throw new Error('bad base58')
    for (let i = 0; i < bytes.length; i += 1) {
      carry += bytes[i] * 58
      bytes[i] = carry & 0xff
      carry >>= 8
    }
    while (carry > 0) {
      bytes.push(carry & 0xff)
      carry >>= 8
    }
  }
  const out = new Uint8Array(zeroes + bytes.length)
  for (let i = 0; i < bytes.length; i += 1) out[out.length - 1 - i] = bytes[i]
  return out
}

export function slipZero(level) {
  let node = hashv([new TextEncoder().encode('slip-empty')])
  for (let i = 0; i < level; i += 1) node = hashv([node, node])
  return node
}

export function slipLeaf(index, owner, combinations, commitment) {
  const ownerBytes = typeof owner === 'string' ? decodeBase58(owner) : owner
  const commitmentBytes = typeof commitment === 'string' ? hexToBytes(commitment) : commitment
  if (ownerBytes.length !== 32 || commitmentBytes.length !== 32) throw new Error('bad slip leaf')
  return hashv([
    new TextEncoder().encode('slip-leaf'),
    u32le(index),
    ownerBytes,
    u32le(combinations),
    commitmentBytes,
  ])
}

export function slipMark () {
  return hashv([new TextEncoder().encode('slip-marked')])
}

export function slipRevealLeaf (index, owner, combinations, commitment, picks) {
  const ownerBytes = typeof owner === 'string' ? decodeBase58(owner) : owner
  const commitmentBytes = typeof commitment === 'string' ? hexToBytes(commitment) : commitment
  const padded = new Uint8Array(13)
  padded.set(picks instanceof Uint8Array ? picks.subarray(0, 13) : Uint8Array.from(picks).subarray(0, 13))
  return hashv([
    new TextEncoder().encode('slip-reveal'),
    u32le(index),
    ownerBytes,
    u32le(combinations),
    commitmentBytes,
    padded
  ])
}

function nodeAt (level, index, depth) {
  if (index < level.length && level[index]) return level[index]
  return slipZero(depth)
}

/** Sibling path for one index. A missing node is an empty leaf, so reveals and claims can arrive in any order. */
export function sparseProof (nodes, index) {
  if (!Number.isInteger(index) || index < 0 || index >= 2 ** SLIP_DEPTH) throw new Error('that coupon is not in the archive')
  let level = nodes
  let cursor = index
  const proof = []
  for (let depth = 0; depth < SLIP_DEPTH; depth += 1) {
    proof.push(nodeAt(level, cursor ^ 1, depth))
    const nextCount = Math.max(Math.ceil(level.length / 2), (cursor >> 1) + 1)
    const next = []
    for (let i = 0; i < nextCount; i += 1) {
      const leftMissing = !(i * 2 < level.length && level[i * 2])
      const rightMissing = !(i * 2 + 1 < level.length && level[i * 2 + 1])
      next.push(leftMissing && rightMissing ? null : hashv([nodeAt(level, i * 2, depth), nodeAt(level, i * 2 + 1, depth)]))
    }
    level = next
    cursor >>= 1
  }
  return proof
}

export function slipFold(index, leaf, proof) {
  let node = leaf
  let cursor = index
  for (const sibling of proof) {
    node = cursor % 2 === 0 ? hashv([node, sibling]) : hashv([sibling, node])
    cursor >>= 1
  }
  return node
}

/** Sibling path for one coupon in a left-filled tree. Empty siblings match the program. */
export function proofFor(leaves, index) {
  if (!Number.isInteger(index) || index < 0 || index >= leaves.length) {
    throw new Error('that coupon is not in the archive')
  }
  let level = leaves.map((leaf) => leaf)
  let cursor = index
  const proof = []
  for (let depth = 0; depth < SLIP_DEPTH; depth += 1) {
    const sibling = cursor ^ 1
    proof.push(sibling < level.length ? level[sibling] : slipZero(depth))
    const next = []
    for (let i = 0; i < level.length; i += 2) {
      const right = i + 1 < level.length ? level[i + 1] : slipZero(depth)
      next.push(hashv([level[i], right]))
    }
    level = next
    cursor >>= 1
  }
  return proof
}

export function hexToBytes(hex) {
  if (hex.length % 2 !== 0) throw new Error('bad hex')
  const bytes = new Uint8Array(hex.length / 2)
  for (let i = 0; i < bytes.length; i += 1) bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  return bytes
}
