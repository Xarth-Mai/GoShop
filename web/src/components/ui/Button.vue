<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'secondary-on-dark' | 'text'
    disabled?: boolean
    loading?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  {
    variant: 'primary',
    disabled: false,
    loading: false,
    type: 'button'
  }
)

const className = computed(() => {
  if (props.variant === 'text') return 'btn-text'
  return `btn-${props.variant}`
})
</script>

<template>
  <button
    :type="type"
    :class="[className, { 'btn-loading': loading }]"
    :disabled="disabled || loading"
  >
    <svg v-if="loading" class="spinner" viewBox="0 0 50 50">
      <circle class="path" cx="25" cy="25" r="20" fill="none" stroke-width="5"></circle>
    </svg>
    <slot v-else></slot>
  </button>
</template>

<style scoped>
.btn-text {
  background: none;
  border: none;
  color: var(--colors-primary);
  font-size: 14px;
  font-weight: 500;
  padding: 0;
  height: auto;
}
.btn-text:hover:not(:disabled) {
  color: var(--colors-primary-active);
  text-decoration: underline;
}
.btn-text:disabled {
  color: var(--colors-muted-soft);
  cursor: not-allowed;
}

.spinner {
  animation: rotate 2s linear infinite;
  width: 18px;
  height: 18px;
}

.spinner .path {
  stroke: currentColor;
  stroke-linecap: round;
  animation: dash 1.5s ease-in-out infinite;
}

@keyframes rotate {
  100% {
    transform: rotate(360deg);
  }
}

@keyframes dash {
  0% {
    stroke-dasharray: 1, 150;
    stroke-dashoffset: 0;
  }
  50% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -35;
  }
  100% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -124;
  }
}
</style>
