// A slip has no coupon account, so the browser flag can lag the keeper.
// The archive is what was actually published.
export function publishedSlipPatch (coupon, archive, wallet) {
  if (!coupon?.slip || !archive || !wallet) return {}
  const same = row => row && row.owner === wallet && Number(row.index) === Number(coupon.index)
  const patch = {}
  if (!coupon.revealed && (archive.reveals || []).some(same)) patch.revealed = true
  if (!coupon.claimed && (archive.claims || []).some(same)) patch.claimed = true
  if (!coupon.refunded && (archive.refunds || []).some(same)) patch.refunded = true
  return patch
}
