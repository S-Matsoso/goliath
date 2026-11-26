<template>
  <main class="login-page">
    <section class="login-brand-panel">
      <RouterLink class="brand brand--light" to="/login">
        <span class="brand__mark">G</span>
        <span><strong>Goliath</strong><small>PUBLIC REGISTRY</small></span>
      </RouterLink>
      <div class="login-brand-panel__message">
        <p class="eyebrow eyebrow--light">CONNECTED GOVERNMENT SERVICES</p>
        <h1>One trusted record.<br />Services that work.</h1>
        <p>A secure workspace for registry administrators and department officials.</p>
      </div>
      <div class="login-brand-panel__footer">
        <span class="security-seal">✓</span>
        <span>Protected access <span class="login-brand-panel__dot">·</span> Audited activity</span>
      </div>
    </section>

    <section class="login-form-panel">
      <div class="login-form-wrap">
        <div class="login-mobile-brand brand">
          <span class="brand__mark">G</span>
          <span><strong>Goliath</strong><small>PUBLIC REGISTRY</small></span>
        </div>
        <p class="eyebrow">STAFF ACCESS</p>
        <h2>Welcome back</h2>
        <p class="muted login-intro">Sign in with the account provided by your system administrator.</p>

        <form class="form-stack" @submit.prevent="submit">
          <label class="field">
            <span>Username</span>
            <input v-model.trim="username" autocomplete="username" autofocus required placeholder="e.g. jane.doe" />
          </label>
          <label class="field">
            <span>Password</span>
            <input v-model="password" type="password" autocomplete="current-password" required placeholder="Enter your password" />
          </label>
          <p v-if="error" class="form-error" role="alert">{{ error }}</p>
          <button class="button button--primary button--wide" type="submit" :disabled="busy">
            {{ busy ? 'Signing in…' : 'Sign in securely' }}
            <span v-if="!busy" aria-hidden="true">→</span>
          </button>
        </form>

        <div class="login-help">
          <span class="login-help__icon">i</span>
          <p>Need access? Contact your department administrator. Accounts are created by an administrator; self-registration is not available.</p>
        </div>
        <p class="login-privacy">Your session is temporary and your activity is recorded for security.</p>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ApiError, apiRequest } from '../services/api'
import { acceptSession } from '../stores/auth'
import type { Session } from '../types'

const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref(typeof route.query.notice === 'string' ? route.query.notice : '')

async function submit() {
  busy.value = true
  error.value = ''
  try {
    const session = await apiRequest<Session>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username: username.value, password: password.value }),
    })
    acceptSession(session)
    await router.replace(session.must_change_password
      ? { name: 'change-password' }
      : session.principal.role === 'admin'
        ? { name: 'admin-home' }
        : { name: 'workspace' })
  } catch (cause) {
    error.value = cause instanceof ApiError
      ? cause.message
      : 'The service could not be reached. Please try again.'
  } finally {
    busy.value = false
  }
}
</script>
