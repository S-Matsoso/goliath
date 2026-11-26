<template>
  <div class="view">
    <div class="page-heading">
      <div>
        <p class="eyebrow">{{ principal?.department }} / DEPARTMENT SERVICES</p>
        <h1>{{ service?.title ?? 'Service desk' }}</h1>
        <p class="muted">{{ service?.description ?? 'Available registry actions for your department.' }}</p>
      </div>
    </div>

    <template v-if="!selectedOperation">
      <div v-if="operations.length" class="service-grid service-grid--compact">
        <RouterLink
          v-for="operation in operations"
          :key="operation.id"
          class="service-card"
          :to="{ name: 'department-work', query: { operation: operation.id } }"
        >
          <span class="service-card__role">{{ operation.method }} · {{ titleCase(operation.minimumRole) }}</span>
          <strong>{{ operation.title }}</strong>
          <p>{{ operation.description }}</p>
          <span class="service-card__link">Open service <span aria-hidden="true">→</span></span>
        </RouterLink>
      </div>
      <BaseCard v-else title="No services available" eyebrow="DEPARTMENT SERVICES">
        <p class="muted">No service is assigned to this department and role. Contact your system administrator for access.</p>
      </BaseCard>
    </template>

    <template v-else>
      <button class="back-link" @click="backToServices">← All services</button>
      <div class="service-work-grid">
        <BaseCard :title="selectedOperation.title" eyebrow="REGISTRY OPERATION" :description="selectedOperation.description">
          <form class="form-stack" @submit.prevent="runOperation">
            <label v-for="field in selectedOperation.fields" :key="field.key" class="field">
              <span>{{ field.label }}<em v-if="field.required">Required</em></span>
              <select v-if="field.type === 'select'" v-model="values[field.key]" :required="field.required">
                <option disabled value="">Select an option</option>
                <option v-for="option in field.options" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
              <input
                v-else
                v-model="values[field.key]"
                :type="field.type ?? 'text'"
                :required="field.required"
                :placeholder="field.placeholder"
                :autocomplete="field.key === 'national_id' ? 'off' : undefined"
              />
            </label>
            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <button class="button button--primary" :disabled="running">{{ running ? 'Submitting…' : `${selectedOperation.method} · Run operation` }}</button>
          </form>
          <p class="sensitive-note"><span>i</span> Only submit information required for this official purpose. This request is audited.</p>
        </BaseCard>

        <BaseCard title="Response" eyebrow="SERVICE RESULT" description="Results are displayed for this session and are not saved by the frontend.">
          <div v-if="running" class="loading-state">Waiting for the registry…</div>
          <div v-else-if="result" class="response-result">
            <StatusBadge label="Completed" tone="success" />
            <pre>{{ result }}</pre>
          </div>
          <div v-else-if="error" class="response-error"><StatusBadge label="Request failed" tone="danger" /><p>{{ error }}</p></div>
          <div v-else class="response-placeholder"><span>↗</span><p>Submit an operation to see its response here.</p></div>
        </BaseCard>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import BaseCard from '../../components/BaseCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { ApiError, apiRequest } from '../../services/api'
import { authState } from '../../stores/auth'
import { availableOperations, departmentServices } from '../../services/department-services'
import type { DepartmentOperation } from '../../services/department-services'

const route = useRoute()
const router = useRouter()
const principal = computed(() => authState.principal)
const department = computed(() => principal.value?.department ?? '')
const service = computed(() => departmentServices[department.value])
const operations = computed(() => availableOperations(department.value, principal.value?.role ?? ''))
const selectedOperation = computed(() => operations.value.find((operation) => operation.id === route.query.operation))
const values = reactive<Record<string, string>>({})
const running = ref(false)
const error = ref('')
const result = ref('')

watch(selectedOperation, (operation) => {
  for (const key of Object.keys(values)) delete values[key]
  for (const field of operation?.fields ?? []) values[field.key] = ''
  error.value = ''
  result.value = ''
}, { immediate: true })

async function runOperation() {
  const operation = selectedOperation.value
  if (!operation) return
  running.value = true
  error.value = ''
  result.value = ''
  try {
    const path = buildPath(operation, values)
    const body = buildBody(operation, values)
    const response = await apiRequest<unknown>(path, {
      method: operation.method,
      ...(operation.method === 'GET' ? {} : { body: JSON.stringify(body) }),
    })
    result.value = JSON.stringify(response, null, 2)
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'The registry service could not be reached.'
  } finally {
    running.value = false
  }
}

function buildPath(operation: DepartmentOperation, formValues: Record<string, string>) {
  if (operation.queryFields) {
    const query = new URLSearchParams()
    for (const field of operation.fields) {
      if (formValues[field.key]) query.set(field.key, formValues[field.key])
    }
    return query.size ? `${operation.path}?${query.toString()}` : operation.path
  }
  return operation.path.replace(/\{([^}]+)\}/g, (_match, key: string) => encodeURIComponent(formValues[key] ?? ''))
}

function buildBody(operation: DepartmentOperation, formValues: Record<string, string>) {
  const pathParameter = operation.path.match(/\{([^}]+)\}/)?.[1]
  return Object.fromEntries(operation.fields
    .filter((field) => field.key !== pathParameter && formValues[field.key] !== '')
    .map((field) => [
      field.key,
      field.type === 'number' ? Number(formValues[field.key]) : formValues[field.key],
    ]))
}

function backToServices() {
  void router.replace({ name: 'department-work' })
}

function titleCase(value: string) {
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : ''
}
</script>
