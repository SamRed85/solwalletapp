<template>
  <div v-if="picks.length" class="mb-3">
    <div v-if="painted.length" class="color-legend">
      <span v-for="color in painted" :key="color.id" class="legend-item">
        <i class="swatch" :class="color.id" />
        {{ color.label }} Min {{ color.min }} and Max {{ color.max }}
      </span>
    </div>
    <div v-for="(selected, index) in picks" :key="index" class="game-row">
      <div class="game-top">
        <span class="game-line">
          <span class="game-index">{{ index + 1 }}.</span><span class="game-name">{{ label(index) }}</span>
          <MatchScore v-if="settled && live[index]" :game="live[index]" :kickoff="games[index].kickoff" />
        </span>
        <span class="picks">
        <button
          v-for="outcome in outcomes" :key="outcome.value" type="button" tabindex="-1" class="pick"
          :class="pickClass(selected, outcome.value, resultAt(index), index)"
          :style="pickStyle(index, selected, outcome.value)"
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
import { colorView, missedColorGames } from '@/utils/reduce'

const INK = { purple: '#6A1B9A', yellow: '#F9A825', red: '#C62828', green: '#2E7D32' }

const props = defineProps({
  picks: { type: Array, default: () => [] },
  results: { type: String, default: '' },
  games: { type: Array, default: () => [] },
  live: { type: Array, default: () => [] },
  colors: { type: Array, default: () => [] }
})

const painted = computed(() => colorView(props.colors))

const settled = computed(() => props.results.length > 0 && [...props.results].every(mark => '1X2C'.includes(mark)))
const missedGames = computed(() => (settled.value ? missedColorGames(props.colors, props.results) : new Set()))

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

function marksAt (game, outcome) {
  return painted.value.filter(color => color.marks.some(mark => mark.game === game && mark.outcome === outcome))
}

function pickStyle (game, selected, outcome) {
  if (!selected.includes(outcome)) return undefined
  const marks = marksAt(game, outcome)
  if (!marks.length) return undefined
  const first = INK[marks[0].id]
  const text = marks[0].id === 'yellow' && marks.length === 1 ? '#1a1a1a' : '#fff'
  if (marks.length === 1) return { background: first, borderColor: first, color: text }
  return { background: `linear-gradient(135deg, ${first} 50%, ${INK[marks[1].id]} 50%)`, borderColor: 'transparent', color: '#fff' }
}

function pickClass (selected, outcome, result, game) {
  const chosen = selected.includes(outcome)
  const isResult = settled.value && result === outcome
  const missed = isResult && chosen && missedGames.value.has(game)
  return {
    selected: chosen,
    'ring-hit': isResult && chosen && !missed,
    'ring-muted': missed,
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
.color-legend { display: flex; flex-wrap: wrap; gap: 10px 14px; margin-bottom: 8px; font-size: 12px; font-weight: 600; }
.legend-item { display: inline-flex; align-items: center; gap: 6px; }
.swatch { width: 12px; height: 12px; border-radius: 3px; }
.swatch.purple { background: #6A1B9A; }
.swatch.yellow { background: #F9A825; }
.swatch.red { background: #C62828; }
.swatch.green { background: #2E7D32; }
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
.pick.ring-muted {
  box-shadow: 0 0 0 2px #9aa0a6;
  border-color: #9aa0a6;
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
