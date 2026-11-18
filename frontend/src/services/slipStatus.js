// A slip has no coupon account, so the browser flag can lag the keeper.
// The archive is what was actually published.
const sameSlip = (row, coupon, wallet) => row && Number(row.index) === Number(coupon.index) && (!row.owner || row.owner === wallet)

export function publishedSlipPatch (coupon, archive, wallet) {
  if (!coupon?.slip || !archive || !wallet) return {}
  const same = row => sameSlip(row, coupon, wallet)
  const patch = {}
  if (!coupon.revealed && (archive.reveals || []).some(same)) patch.revealed = true
  if (!coupon.claimed && (archive.claims || []).some(same)) patch.claimed = true
  if (!coupon.refunded && (archive.refunds || []).some(same)) patch.refunded = true
  return patch
}

// A slip lives in the server archive, not in a wallet-owned account. A browser that
// did not place it still lists it from the archive. Picks are present only after reveal.
export function slipFromArchive ({ wallet, event, address, row, reveal, claimed, refunded }) {
  if (!row || row.owner !== wallet || row.index == null || !address) return null
  const index = Number(row.index)
  const coupon = {
    eventId: event?.id || '',
    eventTitle: event?.title || '',
    admin: event?.admin,
    nonce: event?.nonce,
    index,
    slip: true,
    address: `slip:${address}:${index}`,
    commitment: row.commitment,
    combinations: Number(row.combinations) || 0,
    revealed: false
  }
  if (row.signature) coupon.signature = row.signature
  if (claimed) coupon.claimed = true
  if (refunded) coupon.refunded = true
  if (reveal && sameSlip(reveal, coupon, wallet) && reveal.picks) {
    coupon.revealed = true
    coupon.picksHex = reveal.picks
    if (reveal.colors) coupon.colorsHex = reveal.colors
  }
  return coupon
}

// The program numbers a coupon when the transaction lands, and logs the number.
// Returns null when the logs have no line for this owner and commitment.
export function slipIndexFromLogs (logs, event, owner, commitment) {
  for (const line of logs || []) {
    const parts = String(line).replace(/^Program log: /, '').split(':')
    if (parts.length !== 6 || parts[0] !== 'slip-v1') continue
    if (parts[1] !== event || parts[3] !== owner || parts[5] !== commitment) continue
    const index = Number(parts[2])
    if (Number.isInteger(index) && index >= 0) return index
  }
  return null
}

// On an event with a canopy a coupon has no number until its transaction lands. Until then
// it is saved under its commitment, so two coupons signed at the same moment never share a key.
export const pendingSlipAddress = (event, commitment) => `slip:${event}:${commitment}`

// False while a slip is still saved under its commitment. Its number is then only a guess.
export function slipNumbered (coupon) {
  return !coupon?.slip || String(coupon.address || '').endsWith(`:${coupon.index}`)
}
