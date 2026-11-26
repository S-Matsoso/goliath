<template>
  <div class="app-shell">
    <aside class="sidebar">
      <RouterLink class="brand" :to="homeLink">
        <span class="brand__mark">G</span>
        <span><strong>Goliath</strong><small>PUBLIC REGISTRY</small></span>
      </RouterLink>

      <div class="sidebar__section-label">{{ isAdmin ? 'ADMINISTRATION' : 'WORKSPACE' }}</div>
      <nav class="sidebar__nav" aria-label="Primary navigation">
        <template v-if="isAdmin">
          <RouterLink class="nav-link" :to="{ name: 'admin-home' }">
            <span class="nav-link__icon">◫</span> Overview
          </RouterLink>
          <RouterLink class="nav-link" :to="{ name: 'admin-users' }">
            <span class="nav-link__icon">♙</span> User directory
          </RouterLink>
          <RouterLink class="nav-link" :to="{ name: 'admin-departments' }">
            <span class="nav-link__icon">▦</span> Departments
          </RouterLink>
        </template>
        <template v-else>
          <RouterLink class="nav-link" :to="{ name: 'workspace' }">
            <span class="nav-link__icon">◫</span> My workspace
          </RouterLink>
          <RouterLink class="nav-link" :to="{ name: 'department-work' }">
            <span class="nav-link__icon">⌕</span> Department services
          </RouterLink>
        </template>
      </nav>

      <div class="sidebar__bottom">
        <div class="security-note">
          <span class="security-note__icon">✓</span>
          <div><strong>Secure workspace</strong><small>Activity is audited</small></div>
        </div>
        <button class="nav-link nav-link--button" @click="changePassword">
          <span class="nav-link__icon">⌑</span> Change password
        </button>
      </div>
    </aside>

    <div class="main-column">
      <header class="topbar">
        <div class="topbar__breadcrumb">
          <span>Registry operations</span>
          <span class="breadcrumb-separator">/</span>
          <strong>{{ pageTitle }}</strong>
        </div>
        <div class="topbar__user">
          <div class="user-avatar">{{ initials }}</div>
          <div class="user-details">
            <strong>{{ principal?.display_name }}</strong>
            <span>{{ isAdmin ? 'System administrator' : `${principal?.department} · ${principal?.role}` }}</span>
          </div>
          <button class="button button--quiet button--small" :disabled="signingOut" @click="logout">
            {{ signingOut ? 'Signing out…' : 'Sign out' }}
          </button>
        </div>
      </header>
      <main class="page-content">
        <RouterView />
      </main>
      <footer class="app-footer">Goliath Registry <span>•</span> Restricted system · Authorized personnel only</footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { authState, signOut } from '../stores/auth'

const router = useRouter()
const route = useRoute()
const principal = computed(() => authState.principal)
const isAdmin = computed(() => principal.value?.role === 'admin')
const homeLink = computed(() => isAdmin.value ? { name: 'admin-home' } : { name: 'workspace' })
const signingOut = ref(false)
const pageTitle = computed(() => {
  const titles: Record<string, string> = {
    'admin-home': 'System overview',
    'admin-users': 'User directory',
    'admin-departments': 'Departments',
    workspace: 'My workspace',
    'department-work': 'Department services',
    'change-password': 'Change password',
  }
  return titles[String(route.name)] ?? 'Workspace'
})
const initials = computed(() => principal.value?.display_name
  .split(/\s+/)
  .slice(0, 2)
  .map((part) => part[0]?.toUpperCase() ?? '')
  .join('') ?? 'U')

function changePassword() {
  void router.push({ name: 'change-password' })
}

async function logout() {
  signingOut.value = true
  try {
    await signOut()
    await router.replace({ name: 'login' })
  } catch (error) {
    signingOut.value = false
    const message = error instanceof Error ? error.message : 'The server did not confirm sign-out.'
    await router.replace({
      name: 'login',
      query: { notice: `This browser session was cleared, but the server could not revoke it: ${message}` },
    })
  }
}
</script>
