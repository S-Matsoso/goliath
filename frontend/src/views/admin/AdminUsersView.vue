<template>
  <div class="view">
    <div class="page-heading page-heading--spread">
      <div>
        <p class="eyebrow">ADMINISTRATION / ACCESS CONTROL</p>
        <h1>User directory</h1>
        <p class="muted">Provision staff accounts and manage their department access.</p>
      </div>
      <button class="button button--primary" @click="openCreate">
        <span aria-hidden="true">＋</span> Create user
      </button>
    </div>

    <div v-if="error" class="notice notice--error" role="alert">{{ error }}</div>
    <BaseCard title="Staff accounts" eyebrow="ACCESS DIRECTORY" :description="`${users.length} accounts · Credentials are issued by an administrator`">
      <div v-if="loading" class="loading-state">Loading user directory…</div>
      <div v-else-if="users.length === 0" class="empty-state">No staff accounts have been provisioned yet.</div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Official</th>
              <th>Department</th>
              <th>Role</th>
              <th>Last sign in</th>
              <th>Status</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.id">
              <td>
                <div class="identity-cell">
                  <span class="user-avatar user-avatar--table">{{ initials(user.display_name) }}</span>
                  <span><strong>{{ user.display_name }}</strong><small>@{{ user.username }}</small></span>
                </div>
              </td>
              <td><span class="department-label">{{ user.department_name }}</span><small class="table-secondary">{{ user.department }}</small></td>
              <td><span class="role-pill">{{ titleCase(user.role) }}</span></td>
              <td>{{ user.last_login_at ? formatDate(user.last_login_at) : 'Never' }}</td>
              <td>
                <div class="status-stack">
                  <StatusBadge :label="user.active ? 'Active' : 'Inactive'" :tone="user.active ? 'success' : 'neutral'" />
                  <small v-if="user.must_change_password" class="status-note">Password change required</small>
                </div>
              </td>
              <td class="table-action">
                <button class="button button--small button--secondary" :disabled="savingId === user.id" @click="toggleUser(user)">
                  {{ savingId === user.id ? 'Saving…' : user.active ? 'Deactivate' : 'Activate' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </BaseCard>

    <div v-if="showCreate" class="modal-backdrop" @click.self="closeCreate">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="create-user-title">
        <header class="modal__header">
          <div><p class="eyebrow">NEW ACCOUNT</p><h2 id="create-user-title">Provision a user</h2></div>
          <button class="icon-button" aria-label="Close dialog" @click="closeCreate">×</button>
        </header>
        <template v-if="!credential">
          <p class="muted">The user will receive a one-time temporary password and must change it at first sign-in.</p>
          <form class="form-stack" @submit.prevent="createUser">
            <label class="field"><span>Username</span><input v-model.trim="form.username" required minlength="3" maxlength="64" autocomplete="off" placeholder="e.g. jane.doe" /><small>Lowercase letters, numbers, dots, underscores or hyphens.</small></label>
            <label class="field"><span>Full name</span><input v-model.trim="form.display_name" required maxlength="120" autocomplete="off" placeholder="Official's full name" /></label>
            <label class="field"><span>Department</span>
              <select v-model="form.department" required>
                <option disabled value="">Select a department</option>
                <option v-for="department in activeDepartments" :key="department.code" :value="department.code">{{ department.name }}</option>
              </select>
            </label>
            <label class="field"><span>Role</span>
              <select v-model="form.role" required>
                <option value="clerk">Clerk</option>
                <option value="supervisor">Supervisor</option>
                <option value="auditor">Auditor</option>
              </select>
              <small>Administrator privileges can only be established during system bootstrap.</small>
            </label>
            <p v-if="modalError" class="form-error" role="alert">{{ modalError }}</p>
            <footer class="modal__footer">
              <button type="button" class="button button--secondary" @click="closeCreate">Cancel</button>
              <button class="button button--primary" :disabled="creating || !activeDepartments.length">{{ creating ? 'Creating…' : 'Create account' }}</button>
            </footer>
          </form>
        </template>
        <div v-else class="credential-reveal">
          <div class="credential-reveal__icon">✓</div>
          <h3>Account ready</h3>
          <p class="muted">Share these sign-in details with <strong>{{ credential.user.display_name }}</strong> using an approved secure channel. The password cannot be viewed again after closing this dialog.</p>
          <div class="credential-field"><span>Username</span><strong>{{ credential.user.username }}</strong></div>
          <div class="credential-field credential-field--secret"><span>Temporary password</span><code>{{ credential.temporary_password }}</code></div>
          <p class="credential-warning">This temporary password is shown once. The user must change it at first sign-in.</p>
          <p v-if="copyError" class="form-error" role="alert">{{ copyError }}</p>
          <footer class="modal__footer">
            <button class="button button--secondary" @click="copyCredentials">{{ copied ? 'Copied' : 'Copy details' }}</button>
            <button class="button button--primary" @click="closeCreate">Done</button>
          </footer>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import BaseCard from '../../components/BaseCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { ApiError, apiRequest } from '../../services/api'
import type { Department, ManagedUser } from '../../types'

interface CreatedCredential {
  user: ManagedUser
  temporary_password: string
}

const users = ref<ManagedUser[]>([])
const departments = ref<Department[]>([])
const loading = ref(true)
const creating = ref(false)
const savingId = ref<number | null>(null)
const showCreate = ref(false)
const error = ref('')
const modalError = ref('')
const copyError = ref('')
const copied = ref(false)
const credential = ref<CreatedCredential | null>(null)
const activeDepartments = computed(() => departments.value.filter((department) => department.active && department.code !== 'ADMIN'))
const form = reactive({ username: '', display_name: '', department: '', role: 'clerk' })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [userPayload, departmentPayload] = await Promise.all([
      apiRequest<{ users: ManagedUser[] }>('/admin/users'),
      apiRequest<{ departments: Department[] }>('/admin/departments'),
    ])
    users.value = userPayload.users
    departments.value = departmentPayload.departments
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'Could not load user accounts.'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.username = ''
  form.display_name = ''
  form.department = activeDepartments.value[0]?.code ?? ''
  form.role = 'clerk'
  modalError.value = ''
  credential.value = null
  copied.value = false
  showCreate.value = true
}

function closeCreate() {
  showCreate.value = false
  credential.value = null
}

async function createUser() {
  creating.value = true
  modalError.value = ''
  try {
    credential.value = await apiRequest<CreatedCredential>('/admin/users', {
      method: 'POST',
      body: JSON.stringify(form),
    })
    await load()
  } catch (cause) {
    modalError.value = cause instanceof ApiError ? cause.message : 'Could not create this account.'
  } finally {
    creating.value = false
  }
}

async function toggleUser(user: ManagedUser) {
  savingId.value = user.id
  error.value = ''
  try {
    await apiRequest(`/admin/users/${user.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ active: !user.active }),
    })
    await load()
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'Could not update this account.'
  } finally {
    savingId.value = null
  }
}

async function copyCredentials() {
  if (!credential.value) return
  copyError.value = ''
  try {
    await navigator.clipboard.writeText(
      `Username: ${credential.value.user.username}\nTemporary password: ${credential.value.temporary_password}`,
    )
    copied.value = true
  } catch {
    copyError.value = 'Clipboard access was denied. Select and copy the credentials manually.'
  }
}

function initials(name: string) {
  return name.split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('')
}

function titleCase(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

onMounted(() => void load())
</script>
