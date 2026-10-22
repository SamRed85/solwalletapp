<template>
  <div class="d-flex flex-wrap ga-1">
    <v-chip
      v-for="(pick, index) in picks" :key="index" size="small" label variant="flat"
      :color="colorFor(index)" :title="titleFor(index)"
    >
      {{ pick }}
    </v-chip>
  </div>
</template>

<script setup>
const props = defineProps({
  coupon: { type: String, required: true },
  games: { type: Array, default: () => [] }
})

const picks = props.coupon.split('')

function colorFor (index) {
  const result = props.games[index]?.result
  if (!result) return 'secondary'
  return picks[index] === result ? 'success' : 'error'
}

function titleFor (index) {
  const game = props.games[index]
  if (!game) return ''
  const result = game.result ? `, result ${game.result}${game.cancelled ? ' (cancelled, drawn)' : ''}` : ''
  return `${index + 1}. ${game.homeTeam} vs ${game.awayTeam}${result}`
}
</script>
