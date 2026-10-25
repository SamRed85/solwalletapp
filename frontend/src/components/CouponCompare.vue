<template>
  <div v-if="picks.length" class="mb-3">
    <div v-for="(selected, index) in picks" :key="index" class="game-row">
      <span class="game-index">{{ index + 1 }}</span>
      <span class="game-name">{{ label(index) }}</span>
      <span v-if="settled" class="d-inline-flex align-center ga-2">
        <MatchScore v-if="live[index]" :game="live[index]" :kickoff="games[index]?.kickoff" />
      </span>
      <span class="picks">
        <button
          v-for="outcome in outcomes" :key="outcome.value" type="button" tabindex="-1" class="pick"
          :class="pickClass(selected, outcome.value, resultAt(index))"
        >
          {{ outcome.value }}
        </button>
      </span>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import MatchScore from '@/components/MatchScore.vue'
import { outcomes } from '@/utils/format'

const props = defineProps({
  picks: { type: Array, default: () => [] },
  results: { type: String, default: '' },
  games: { type: Array, default: () => [] },
  live: { type: Array, default: () => [] }
})

const settled = computed(() => props.results.length > 0 && [...props.results].every(mark => '1X2C'.includes(mark)))

function resultAt (index) {
  return props.results[index] || ''
}

function label (index) {
  const game = props.games[index]
  return game?.homeTeam ? `${game.homeTeam} - ${game.awayTeam}` : 'Game'
}

function covered (selected, result) {
  if (!result) return false
  return result === 'C' || selected.includes(result)
}

function pickClass (selected, outcome, result) {
  const chosen = selected.includes(outcome)
  const isResult = settled.value && result === outcome
  return {
    selected: chosen,
    'ring-hit': isResult && chosen,
    'ring-miss': isResult && !chosen
  }
}
</script>

<style scoped>
.game-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 4px;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.game-index { width: 22px; font-weight: 700; }
.game-name { flex: 1 1 140px; font-weight: 600; }
.result-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 26px;
  padding: 0 8px;
  border-radius: 8px;
  color: #fff;
  font-weight: 700;
  font-size: 13px;
}
.result-hit { background: #2e9e4f; }
.result-miss { background: #e23b3b; }
.picks { display: inline-flex; gap: 6px; }
.pick {
  min-width: 40px;
  height: 36px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid rgba(var(--v-border-color), 0.6);
  background: rgb(var(--v-theme-surface));
  color: rgba(var(--v-theme-on-surface), var(--v-high-emphasis-opacity));
  font: inherit;
  font-weight: 600;
  cursor: default;
}
.pick.selected {
  background: #163a66;
  border-color: #163a66;
  color: #fff;
}
.pick.ring-hit { 
  box-shadow: 0 0 0 2px #2e9e4f; 
  border-color: #2e9e4f;
}
.pick.ring-miss { 
  box-shadow: 0 0 0 3px #e23b3b;
  border-color: #e23b3b;
}
</style>
