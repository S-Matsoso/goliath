<template>
  <span class="status-badge" :class="`status-badge--${tone}`">
    <span class="status-badge__dot" />
    {{ label }}
  </span>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ label: string; tone?: 'success' | 'warning' | 'danger' | 'neutral' }>(), {
  tone: 'neutral',
})
</script>
