<template>
  <span class="sol-amount" :class="{ 'is-fading': fading }">{{ formatSol(lamports) }}</span>
</template>

<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { display } from '@/stores/currency'
import { formatSol } from '@/utils/format'

defineProps({
  lamports: { type: Number, required: true }
})

const fading = ref(false)
let frame

watch(() => display.currency, async () => {
  fading.value = false
  await nextTick()
  frame = requestAnimationFrame(() => { fading.value = true })
})

onBeforeUnmount(() => cancelAnimationFrame(frame))
</script>
