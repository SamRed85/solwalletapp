<template>
  <div class="mb-2">
    <div class="text-body-2 mb-1">Best row: {{ best }} of {{ games }} correct</div>
    <v-table v-if="payouts && lines.length" density="compact">
      <thead>
        <tr>
          <th>Correct</th>
          <th>Your rows</th>
          <th>Each row wins</th>
          <th>Total</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="line in lines" :key="line.correct">
          <td>{{ line.correct }} of {{ games }}</td>
          <td>{{ Number(line.rows).toLocaleString() }}</td>
          <td>{{ formatSol(Number(line.each)) }}</td>
          <td>{{ formatSol(Number(lineTotal(line))) }}</td>
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

function lineTotal (line) {
  const each = typeof line.each === 'bigint' ? line.each : BigInt(Math.round(Number(line.each)))
  return each * BigInt(line.rows)
}
</script>
