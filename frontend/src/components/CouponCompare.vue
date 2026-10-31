<template>
  <div v-if="picks.length" class="mb-3">
    <div v-for="(selected, index) in picks" :key="index" class="game-row">
      <div class="game-top">
        <span class="game-line">
          <span class="game-index">{{ index + 1 }}.</span><span class="game-name">{{ label(index) }}</span>
          <MatchScore v-if="settled && live[index]" :game="live[index]" :kickoff="games[index].kickoff" />
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
      <span v-if="games[index]?.kickoff" class="game-when">{{ kickoffStamp(games[index].kickoff) }}</span>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import MatchScore from '@/components/MatchScore.vue'
import { kickoffStamp } from '@/services/projection'
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
  flex-direction: column;
  align-items: stretch;
  gap: 1px;
  width: 100%;
  padding: 8px 4px;
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.game-top { display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0; }
.game-line { display: flex; align-items: center; gap: 6px; flex: 1 1 auto; max-width: 100%; min-width: 0; }
.game-line :deep(.when) { display: none; }
.game-line :deep(.match-score) { flex: none; gap: 4px; }
.game-line :deep(.clock) { min-width: 0; font-size: 11px; }
.game-line :deep(.pill) { min-width: 0; padding: 1px 6px; font-size: 11px; }
.game-index { flex: none; margin-right: 3px; font-weight: 700; }
.game-when {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  line-height: 1.1;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.62);
  white-space: nowrap;
}
.game-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-weight: 600;
  font-size: 13px;
}
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
.picks { display: inline-flex; gap: 8px; margin-left: auto; }
.pick {
  min-width: 25px;
  width: 25px;
  height: 25px;
  flex: none;
  padding: 0;
  border-radius: 6px;
  border: 1px solid rgba(var(--v-border-color), 0.6);
  background: rgb(var(--v-theme-surface));
  color: rgba(var(--v-theme-on-surface), var(--v-high-emphasis-opacity));
  font: inherit;
  font-size: 12px;
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
@media (max-width: 959px) {
  .game-index { font-size: 10px; }
  .game-name { font-size: 10px; }
  .game-when { font-size: 9px; }
}
</style>
