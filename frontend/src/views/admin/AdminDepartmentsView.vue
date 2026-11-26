<template>
  <div class="view">
    <div class="page-heading page-heading--spread">
      <div>
        <p class="eyebrow">ADMINISTRATION / ORGANIZATION</p>
        <h1>Departments</h1>
        <p class="muted">Manage department workspaces and their access status.</p>
      </div>
      <button class="button button--primary" @click="openCreate"><span aria-hidden="true">＋</span> Add department</button>
    </div>

    <div v-if="error" class="notice notice--error" role="alert">{{ error }}</div>
    <div class="department-grid">
      <BaseCard v-for="department in departments" :key="department.code" class="department-card">
        <div class="department-card__top">
          <span class="department-icon">▦</span>
          <StatusBadge :label="department.active ? 'Active' : 'Inactive'" :tone="department.active ? 'success' : 'neutral'" />
        </div>
        <p class="eyebrow">{{ department.code }}</p>
        <h2>{{ department.name }}</h2>
        <p class="department-card__members"><strong>{{ department.member_count }}</strong> {{ department.member_count === 1 ? 'official' : 'officials' }}</p>
        <div class="department-card__actions">
          <button class="button button--small button--secondary" @click="openEdit(department)">Edit details</button>
          <button class="button button--small button--quiet" :disabled="savingCode === department.code || department.code === 'ADMIN'" @click="toggleDepartment(department)">
            {{ savingCode === department.code ? 'Saving…' : department.active ? 'Deactivate' : 'Activate' }}
          </button>
        </div>
        <p v-if="department.code === 'ADMIN'" class="system-department-note">System department · protected</p>
      </BaseCard>
      <div v-if="loading" class="loading-state">Loading departments…</div>
      <div v-else-if="!departments.length" class="empty-state">No departments yet. Add one to start provisioning users.</div>
    </div>

    <div v-if="showModal" class="modal-backdrop" @click.self="closeModal">
      <section class="modal modal--narrow" role="dialog" aria-modal="true" :aria-labelledby="editing ? 'edit-department-title' : 'new-department-title'">
        <header class="modal__header">
          <div><p class="eyebrow">ORGANIZATION</p><h2 :id="editing ? 'edit-department-title' : 'new-department-title'">{{ editing ? 'Edit department' : 'Add department' }}</h2></div>
          <button class="icon-button" aria-label="Close dialog" @click="closeModal">×</button>
        </header>
        <form class="form-stack" @submit.prevent="saveDepartment">
          <label v-if="!editing" class="field"><span>Department code</span><input v-model.trim="form.code" required maxlength="20" placeholder="e.g. HEALTH" /><small>Uppercase letters, numbers, underscores, and hyphens.</small></label>
          <div v-else class="field"><span>Department code</span><div class="readonly-field">{{ editing.code }}</div></div>
          <label class="field"><span>Department name</span><input v-model.trim="form.name" required maxlength="100" placeholder="Department of Health" /></label>
          <label v-if="editing" class="checkbox-field"><input v-model="form.active" type="checkbox" :disabled="editing.code === 'ADMIN'" /><span>Department is active</span></label>
          <p v-if="modalError" class="form-error" role="alert">{{ modalError }}</p>
          <footer class="modal__footer">
            <button type="button" class="button button--secondary" @click="closeModal">Cancel</button>
            <button class="button button--primary" :disabled="saving">{{ saving ? 'Saving…' : editing ? 'Save changes' : 'Create department' }}</button>
          </footer>
        </form>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import BaseCard from '../../components/BaseCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { ApiError, apiRequest } from '../../services/api'
import type { Department } from '../../types'

const departments = ref<Department[]>([])
const loading = ref(true)
const saving = ref(false)
const savingCode = ref<string | null>(null)
const showModal = ref(false)
const editing = ref<Department | null>(null)
const error = ref('')
const modalError = ref('')
const form = reactive({ code: '', name: '', active: true })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await apiRequest<{ departments: Department[] }>('/admin/departments')
    departments.value = response.departments
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'Could not load departments.'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.code = ''
  form.name = ''
  form.active = true
  modalError.value = ''
  showModal.value = true
}

function openEdit(department: Department) {
  editing.value = department
  form.code = department.code
  form.name = department.name
  form.active = department.active
  modalError.value = ''
  showModal.value = true
}

function closeModal() {
  showModal.value = false
}

async function saveDepartment() {
  saving.value = true
  modalError.value = ''
  try {
    if (editing.value) {
      await apiRequest(`/admin/departments/${encodeURIComponent(editing.value.code)}`, {
        method: 'PATCH',
        body: JSON.stringify({ name: form.name, active: form.active }),
      })
    } else {
      await apiRequest('/admin/departments', {
        method: 'POST',
        body: JSON.stringify({ code: form.code.toUpperCase(), name: form.name }),
      })
    }
    closeModal()
    await load()
  } catch (cause) {
    modalError.value = cause instanceof ApiError ? cause.message : 'Could not save this department.'
  } finally {
    saving.value = false
  }
}

async function toggleDepartment(department: Department) {
  savingCode.value = department.code
  error.value = ''
  try {
    await apiRequest(`/admin/departments/${encodeURIComponent(department.code)}`, {
      method: 'PATCH',
      body: JSON.stringify({ active: !department.active }),
    })
    await load()
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'Could not update this department.'
  } finally {
    savingCode.value = null
  }
}

onMounted(() => void load())
</script>
