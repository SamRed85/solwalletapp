import { reactive, watch } from 'vue'

const KEY = 'game-card-details'

function loadCardDetails () {
  const base = { odds: false, probability: false, startTime: true, pickShare: false }
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) || 'null')
    if (!saved || typeof saved !== 'object') return base
    return {
      odds: !!saved.odds,
      probability: !!saved.probability,
      startTime: saved.startTime !== false,
      pickShare: !!saved.pickShare
    }
  } catch {
    return base
  }
}

export const cardDetails = reactive(loadCardDetails())

watch(cardDetails, value => {
  localStorage.setItem(KEY, JSON.stringify({
    odds: value.odds,
    probability: value.probability,
    startTime: value.startTime,
    pickShare: value.pickShare
  }))
})

export function eventStarted (event, now = new Date()) {
  const start = new Date(event?.startTime)
  return !Number.isNaN(start.getTime()) && start.getTime() <= now.getTime()
}
