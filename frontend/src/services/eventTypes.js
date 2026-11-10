// The three event types. The kind byte is what the program stores, and it is also
// how rollover finds the next event. 1 is reserved for a league event.
export const EVENT_TYPES = {
  saturday: { id: 'saturday', name: 'Saturday 13', short: 'Saturday', kind: 0, games: 13, minGames: 13, maxGames: 13, locked: true, icon: 'mdi-star', color: 'primary' },
  wildcard: { id: 'wildcard', name: 'Wildcard 13', short: 'Wildcard', kind: 2, games: 13, minGames: 13, maxGames: 13, locked: true, icon: 'mdi-lightning-bolt', color: '#2DD4BF' },
  sprint: { id: 'sprint', name: 'Sprint 8', short: 'Sprint', kind: 3, games: 8, minGames: 7, maxGames: 12, locked: false, icon: 'mdi-dice-5' }
}

export const EVENT_TYPE_LIST = [EVENT_TYPES.saturday, EVENT_TYPES.wildcard, EVENT_TYPES.sprint]

export function eventTypeOf (event) {
  return EVENT_TYPES[event?.type] || EVENT_TYPES.saturday
}

export function typeFromKind (kind) {
  return EVENT_TYPE_LIST.find(item => item.kind === kind) || null
}

// League events pay with RAD. Their kind stays 1, whatever type the form shows.
export function kindOf (event) {
  if (event?.league) return 1
  return eventTypeOf(event).kind
}
