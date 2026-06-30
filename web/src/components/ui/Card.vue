<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'cream' | 'dark'
    hoverable?: boolean
  }>(),
  {
    variant: 'cream',
    hoverable: false
  }
)

const className = computed(() => {
  return [
    `card-${props.variant}`,
    { 'card-hoverable': props.hoverable }
  ]
})
</script>

<template>
  <div :class="className">
    <slot></slot>
  </div>
</template>

<style scoped>
.card-hoverable {
  transition: transform 0.2s cubic-bezier(0.16, 1, 0.3, 1), box-shadow 0.2s ease;
}
.card-hoverable:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(20, 20, 19, 0.05);
  cursor: pointer;
}
</style>
