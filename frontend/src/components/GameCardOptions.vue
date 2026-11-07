<template>
  <div class="card-options">
    <div class="option-caption">Show on game cards</div>
    <div class="option-grid">
      <v-checkbox v-model="cardDetails.odds" label="Odds" density="compact" hide-details class="card-check" />
      <v-checkbox v-model="cardDetails.probability" label="Probability" density="compact" hide-details class="card-check" />
      <v-checkbox v-model="cardDetails.startTime" label="Start time" density="compact" hide-details class="card-check" />
      <v-checkbox v-model="cardDetails.pickShare" label="Pick share (after start)" density="compact" hide-details class="card-check" />
    </div>
    <div v-if="cardDetails.pickShare && started && event?.chain && event.pickShare" class="share-note">
      from {{ event.pickShare.revealed }} of {{ event.pickShare.coupons }} coupons revealed
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { cardDetails, eventStarted } from '@/services/gameCards'

const props = defineProps({
  event: { type: Object, default: null }
})

const started = computed(() => eventStarted(props.event))
</script>

<style scoped>
.card-options {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0 0 4px;
}
.option-caption {
  font-size: 10px;
  line-height: 1.2;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.option-grid {
  display: flex;
  flex-wrap: nowrap;
  justify-content: center;
  align-items: center;
  column-gap: 8px;
  max-width: 100%;
}
.card-check { margin: 0; }
.card-check :deep(.v-selection-control) {
  min-height: 18px;
}
.card-check :deep(.v-selection-control__wrapper),
.card-check :deep(.v-selection-control__input) {
  width: 16px;
  height: 16px;
}
.card-check :deep(.v-icon) {
  font-size: 14px;
}
.card-check :deep(.v-label) {
  font-size: 11px;
  margin-inline-start: 2px;
  opacity: 0.85;
}
.share-note {
  font-size: 10px;
  font-weight: 600;
  text-align: center;
  color: rgba(var(--v-theme-on-surface), 0.62);
}
</style>
