// The bar link for the round nearest to today.
// An open event is labeled Next event. One that has started and is not settled is Active event.
export function featuredEvent (events, now = Date.now()) {
  const list = (Array.isArray(events) ? events : []).filter(event => event && (event.status === 'open' || event.status === 'closed'))
  let closest = null
  let closestDistance = Infinity
  for (const event of list) {
    const start = new Date(event.startTime).getTime()
    if (Number.isNaN(start)) continue
    const distance = Math.abs(start - now)
    const laterTie = distance === closestDistance && start > new Date(closest.startTime).getTime()
    if (distance < closestDistance || laterTie) {
      closest = event
      closestDistance = distance
    }
  }
  if (!closest) return null
  const upcoming = closest.status === 'open'
  return {
    title: upcoming ? 'Next event' : 'Active event',
    icon: upcoming ? 'mdi-calendar-arrow-right' : 'mdi-broadcast',
    to: `/events/${closest.id}`
  }
}
