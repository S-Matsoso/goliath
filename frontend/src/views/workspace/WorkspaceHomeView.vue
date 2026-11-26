<template>
  <div class="view">
    <div class="page-heading">
      <div>
        <p class="eyebrow">DEPARTMENT WORKSPACE</p>
        <h1>Good {{ timeOfDay }}, {{ firstName }}</h1>
        <p class="muted">Your secure workspace for {{ departmentName }} operations.</p>
      </div>
    </div>

    <section class="workspace-welcome">
      <div>
        <p class="eyebrow eyebrow--light">SIGNED-IN OFFICIAL</p>
        <h2>{{ principal?.display_name }}</h2>
        <p>{{ departmentName }} <span>·</span> {{ titleCase(principal?.role ?? '') }}</p>
      </div>
      <div class="workspace-welcome__seal">G</div>
    </section>

    <div class="section-heading">
      <div><h2>Department services</h2><p class="muted">Choose an action available to your role.</p></div>
      <RouterLink class="text-link" :to="{ name: 'department-work' }">View all services <span aria-hidden="true">→</span></RouterLink>
    </div>

    <div v-if="operations.length" class="service-grid">
      <RouterLink
        v-for="operation in operations"
        :key="operation.id"
        class="service-card"
        :to="{ name: 'department-work', query: { operation: operation.id } }"
      >
        <span class="service-card__icon">{{ serviceIcon }}</span>
        <span class="service-card__role">{{ titleCase(operation.minimumRole) }} access</span>
        <strong>{{ operation.title }}</strong>
        <p>{{ operation.description }}</p>
        <span class="service-card__link">Open service <span aria-hidden="true">→</span></span>
      </RouterLink>
    </div>
    <BaseCard v-else title="No services are configured" eyebrow="DEPARTMENT SERVICES">
      <p class="muted">Contact your system administrator if you believe this department should have registry access.</p>
    </BaseCard>

    <div class="workspace-footnote"><span class="workspace-footnote__icon">i</span><p>Access is limited to your department and role. Registry actions are recorded in the system audit log.</p></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import BaseCard from '../../components/BaseCard.vue'
import { authState } from '../../stores/auth'
import { availableOperations, departmentServices } from '../../services/department-services'

const principal = computed(() => authState.principal)
const operations = computed(() => availableOperations(principal.value?.department ?? '', principal.value?.role ?? ''))
const departmentName = computed(() => departmentServices[principal.value?.department ?? '']?.title ?? principal.value?.department ?? 'your department')
const firstName = computed(() => principal.value?.display_name.split(/\s+/)[0] ?? 'there')
const serviceIcon = computed(() => principal.value?.department === 'identity' ? '⌕' : principal.value?.department === 'audit' ? '≋' : '✓')
const hour = new Date().getHours()
const timeOfDay = hour < 12 ? 'morning' : hour < 17 ? 'afternoon' : 'evening'

function titleCase(value: string) {
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : ''
}
</script>
