<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'pill' | 'coral'
  }>(),
  {
    variant: 'pill'
  }
)

const className = computed(() => {
  return `badge-${props.variant}`
})
</script>

<template>
  <span :class="className">
    <slot></slot>
  </span>
</template>
