<template>
  <span v-if="linked" class="match-score">
    <span v-if="playing" class="clock" :class="done ? 'clock-done' : 'clock-live'">{{ clock }}</span>
    <span v-else class="when">{{ when }}</span>
    <span class="pill">{{ home }} - {{ away }}</span>
  </span>
  <span v-else-if="when" class="when">{{ when }}</span>
</template>

<script setup>
import { computed } from 'vue'
import { kickoffLabel, matchClock } from '@/services/projection'

const props = defineProps({
  game: { type: Object, default: null },
  kickoff: { type: [String, Number, Date], default: '' }
})

const linked = computed(() => !!props.game?.linked)
const playing = computed(() => props.game?.homeGoals != null && props.game?.awayGoals != null)
const done = computed(() => ['HT', 'FT', 'AET', 'PEN'].includes(props.game?.status))
const clock = computed(() => matchClock(props.game))
const when = computed(() => kickoffLabel(props.kickoff))
const home = computed(() => playing.value ? props.game.homeGoals : 0)
const away = computed(() => playing.value ? props.game.awayGoals : 0)
</script>

<style scoped>
.match-score {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.when {
  font-weight: 700;
  font-size: 15px;
}
.clock {
  min-width: 36px;
  font-weight: 700;
  font-size: 14px;
}
.clock-live { color: #2f6fed; }
.clock-done { color: #8b909a; }
.pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52px;
  padding: 4px 10px;
  border-radius: 8px;
  background: rgba(var(--v-theme-on-surface), 0.08);
  color: rgb(var(--v-theme-on-surface));
  font-weight: 700;
  font-size: 15px;
  letter-spacing: 0.02em;
}
</style>
