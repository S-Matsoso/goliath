<template>
  <div class="view">
    <div class="page-heading">
      <div>
        <p class="eyebrow">ACCOUNT SECURITY</p>
        <h1>{{ forced ? 'Set a new password' : 'Change your password' }}</h1>
        <p class="muted">Choose a unique passphrase that you do not use for another account.</p>
      </div>
    </div>

    <div class="password-layout">
      <BaseCard class="password-card">
        <form class="form-stack" @submit.prevent="submit">
          <label class="field">
            <span>Current password</span>
            <input v-model="currentPassword" type="password" autocomplete="current-password" required />
          </label>
          <label class="field">
            <span>New password</span>
            <input v-model="newPassword" type="password" autocomplete="new-password" required />
          </label>
          <label class="field">
            <span>Confirm new password</span>
            <input v-model="confirmation" type="password" autocomplete="new-password" required />
          </label>
          <p v-if="error" class="form-error" role="alert">{{ error }}</p>
          <p v-if="success" class="form-success" role="status">{{ success }}</p>
          <button class="button button--primary" :disabled="busy || !valid">
            {{ busy ? 'Updating…' : 'Update password' }}
          </button>
        </form>
      </BaseCard>

      <BaseCard title="Password guidance" eyebrow="KEEP YOUR ACCOUNT SAFE">
        <ul class="guidance-list">
          <li><span>15+</span> characters is the minimum for a password-only account.</li>
          <li><span>Unique</span> — do not reuse a password from another service.</li>
          <li><span>Passphrase</span> — several unrelated words are easier to remember.</li>
          <li><span>Private</span> — a password manager is safer than sharing or writing it down.</li>
        </ul>
        <p class="muted small-text">Avoid personal details and your username. The server checks these rules when you save.</p>
      </BaseCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import BaseCard from '../components/BaseCard.vue'
import { ApiError, apiRequest } from '../services/api'
import { acceptSession, authState } from '../stores/auth'
import type { Session } from '../types'

const router = useRouter()
const forced = computed(() => authState.principal?.must_change_password ?? false)
const currentPassword = ref('')
const newPassword = ref('')
const confirmation = ref('')
const busy = ref(false)
const error = ref('')
const success = ref('')
const valid = computed(() =>
  [...newPassword.value].length >= 15 &&
  newPassword.value === confirmation.value &&
  newPassword.value !== currentPassword.value,
)

async function submit() {
  error.value = ''
  success.value = ''
  if (!valid.value) {
    error.value = 'Use at least 15 characters, confirm the new password, and make it different from the current password.'
    return
  }
  busy.value = true
  try {
    const session = await apiRequest<Session>('/auth/change-password', {
      method: 'POST',
      body: JSON.stringify({
        current_password: currentPassword.value,
        new_password: newPassword.value,
      }),
    })
    acceptSession(session)
    success.value = 'Password updated. Your other sessions have been signed out.'
    currentPassword.value = ''
    newPassword.value = ''
    confirmation.value = ''
    await router.replace(session.principal.role === 'admin' ? { name: 'admin-home' } : { name: 'workspace' })
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.message : 'Could not update your password.'
  } finally {
    busy.value = false
  }
}
</script>
