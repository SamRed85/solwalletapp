<template>
  <div class="mb-2">
    <div class="text-body-2 mb-1">Best row: {{ best }}</div>
    <v-table v-if="payouts && lines.length" density="compact">
      <thead>
        <tr>
          <th>Correct</th>
          <th>Each row wins</th>
          <th>Correct rows</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="line in lines" :key="line.correct">
          <td>{{ line.correct }}</td>
          <td>{{ formatSol(Number(line.each)) }}</td>
          <td>{{ rowCount(line) }}</td>
        </tr>
      </tbody>
    </v-table>
  </div>
</template>

<script setup>
import { formatSol } from '@/utils/format'

defineProps({
  best: { type: Number, required: true },
  games: { type: Number, required: true },
  lines: { type: Array, default: () => [] },
  payouts: { type: Boolean, default: true }
})

function rowCount (line) {
  const yours = Number(line.rows).toLocaleString()
  const total = Number(line.total)
  if (!Number.isFinite(total)) return yours
  return `${total.toLocaleString()} (${yours})`
}
</script>

<style scoped>
th, td { white-space: normal; }
</style>
