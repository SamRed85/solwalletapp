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
