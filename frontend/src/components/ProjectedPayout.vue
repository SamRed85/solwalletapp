<template>
  <div v-if="projection" class="mt-3">
    <div class="text-subtitle-2">If these scores stand</div>
    <div v-if="!projection.complete" class="text-caption text-medium-emphasis">
      {{ waiting }}
    </div>
    <template v-else-if="view">
      <div class="text-caption text-medium-emphasis mb-1">
        <template v-if="view.maximum">Maximum if this coupon were the only winner. Other coupons are still hidden.</template>
        <template v-else>Projected from revealed coupons. This is not the settled payout.</template>
      </div>
      <CouponScore :best="view.best" :games="view.games" :lines="view.lines" />
      <div class="text-caption text-medium-emphasis">This is not the settled payout.</div>
    </template>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import CouponScore from '@/components/CouponScore.vue'
import { projectCoupon } from '@/services/projection'

const props = defineProps({
  system: { type: Array, default: () => [] },
  projection: { type: Object, default: null }
})

const view = computed(() => projectCoupon(props.system, props.projection))
const waiting = computed(() => {
  if (props.projection?.games?.some(game => !game.linked)) return 'A match is not linked to a live score, so this payout cannot be shown.'
  return 'The payout appears once every match has a score.'
})
</script>
