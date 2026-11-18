<template>
  <div ref="root" class="reel-window" :class="{ 'is-dragging': dragging && !reducedMotion(), 'is-ready': ready }" :style="windowStyle">
    <div class="reel" :style="reelStyle">
      <div v-for="item in items" :key="keyOf(item)" class="reel-page" ref="pages">
        <slot :item="item" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  current: { type: [String, Number], required: true },
  items: { type: Array, required: true },
  drag: { type: Number, default: 0 },
  dragging: { type: Boolean, default: false }
})

const root = ref(null)
const pages = ref([])
const height = ref(0)
const ready = ref(false)
function reducedMotion () {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function keyOf (item) {
  return item && typeof item === 'object' ? item.id : item
}

const index = computed(() => {
  const found = props.items.findIndex(item => keyOf(item) === props.current)
  return found < 0 ? 0 : found
})

const reelStyle = computed(() => {
  const drag = props.dragging && !reducedMotion() ? props.drag : 0
  return { transform: `translate3d(calc(${-index.value * 100}% + ${drag}px), 0, 0)` }
})

const windowStyle = computed(() => (height.value ? { height: `${height.value}px` } : undefined))

function measure () {
  const nodes = pages.value
  const width = root.value?.clientWidth || 1
  const drag = props.dragging && !reducedMotion() ? props.drag : 0
  const shift = Math.min(1, Math.max(-1, drag / width))
  const here = index.value
  const neighbor = shift < 0 ? here + 1 : shift > 0 ? here - 1 : here
  const from = nodes[here]?.offsetHeight || 0
  const to = nodes[neighbor]?.offsetHeight ?? from
  const next = Math.round(from + (to - from) * Math.abs(shift))
  if (next > 0) height.value = next
}

let observer
function observe () {
  if (!observer) return
  observer.disconnect()
  for (const node of pages.value) if (node) observer.observe(node)
  measure()
}
onMounted(() => {
  observer = new ResizeObserver(() => measure())
  observe()
  requestAnimationFrame(() => { ready.value = true })
})
watch(pages, () => observe(), { flush: 'post' })
onBeforeUnmount(() => observer?.disconnect())

watch([index, () => props.drag, () => props.dragging, () => props.items], () => nextTick(measure))
</script>

<style scoped>
.reel-window {
  overflow: hidden;
}
.reel-window.is-ready {
  transition: height 280ms ease;
}
.reel {
  display: flex;
  align-items: flex-start;
  width: 100%;
  transition: transform 280ms ease;
}
.reel-window.is-dragging,
.reel-window.is-dragging .reel {
  transition: none;
}
.reel-page {
  flex: 0 0 100%;
  min-width: 0;
  padding-bottom: 6px;
}
@media (prefers-reduced-motion: reduce) {
  .reel-window.is-ready,
  .reel { transition: none; }
}
</style>
