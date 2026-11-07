<template>
  <div v-if="cardDetails.startTime" class="game-when">{{ kickoffStamp(game.kickoff) }}</div>
  <div v-if="cardDetails.odds" class="fact-row">
    <span class="fact-label">Odds</span>
    <span class="fact-cell">{{ oddCell('one') }}</span>
    <span class="fact-cell">{{ oddCell('draw') }}</span>
    <span class="fact-cell">{{ oddCell('two') }}</span>
  </div>
  <div v-if="cardDetails.probability" class="fact-row">
    <span class="fact-label">Probability</span>
    <span class="fact-cell">{{ percentCell(probability, 'one') }}</span>
    <span class="fact-cell">{{ percentCell(probability, 'draw') }}</span>
    <span class="fact-cell">{{ percentCell(probability, 'two') }}</span>
  </div>
  <div v-if="cardDetails.pickShare && started" class="fact-row">
    <span class="fact-label">Picked</span>
    <span class="fact-cell">{{ percentCell(share, 'one') }}</span>
    <span class="fact-cell">{{ percentCell(share, 'draw') }}</span>
    <span class="fact-cell">{{ percentCell(share, 'two') }}</span>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { cardDetails } from '@/services/gameCards'
import { kickoffStamp } from '@/services/projection'

const props = defineProps({
  game: { type: Object, required: true },
  share: { type: Object, default: null },
  started: { type: Boolean, default: false }
})

const probability = computed(() => props.game.odds ? {
  one: props.game.odds.onePercent,
  draw: props.game.odds.drawPercent,
  two: props.game.odds.twoPercent
} : null)

function oddCell (key) {
  const value = props.game.odds?.[key]
  return value ? value : '—'
}

function percentCell (row, key) {
  if (!row || (row.one === 0 && row.draw === 0 && row.two === 0)) return '—'
  return `${row[key]}%`
}
</script>

<style scoped>
.game-when {
  grid-column: 1;
  font-size: 11px;
  line-height: 1.2;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.62);
  white-space: nowrap;
}
.fact-row {
  display: grid;
  grid-template-columns: subgrid;
  grid-column: 1 / -1;
  align-items: center;
}
.fact-label {
  font-size: 9px;
  line-height: 1.2;
  font-weight: 600;
  white-space: nowrap;
  text-align: right;
  color: rgba(var(--v-theme-on-surface), 0.45);
}
.fact-cell {
  font-size: 9px;
  line-height: 1.2;
  font-weight: 700;
  text-align: center;
  white-space: nowrap;
  color: rgba(var(--v-theme-on-surface), 0.72);
}
@media (max-width: 959px) {
  .game-when { font-size: 9px; }
}
</style>
