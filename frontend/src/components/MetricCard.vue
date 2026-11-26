<template>
  <BaseCard class="metric-card">
    <div class="metric-card__top">
      <span class="metric-card__label">{{ label }}</span>
      <span class="metric-card__icon" aria-hidden="true">{{ icon }}</span>
    </div>
    <div class="metric-card__value">{{ value }}</div>
    <div class="metric-card__detail">{{ detail }}</div>
  </BaseCard>
</template>

<script setup lang="ts">
import BaseCard from './BaseCard.vue'

defineProps<{
  label: string
  value: string
  detail: string
  icon: string
}>()
</script>
