import { createRouter, createWebHistory } from 'vue-router'
import { authState } from '../stores/auth'
import LoginView from '../views/LoginView.vue'
import PasswordChangeView from '../views/PasswordChangeView.vue'
import AdminDashboardView from '../views/admin/AdminDashboardView.vue'
import AdminUsersView from '../views/admin/AdminUsersView.vue'
import AdminDepartmentsView from '../views/admin/AdminDepartmentsView.vue'
import WorkspaceHomeView from '../views/workspace/WorkspaceHomeView.vue'
import DepartmentWorkView from '../views/workspace/DepartmentWorkView.vue'
import AppShell from '../components/AppShell.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: AppShell,
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'home', component: WorkspaceHomeView },
        { path: 'admin', name: 'admin-home', component: AdminDashboardView, meta: { admin: true } },
        { path: 'admin/users', name: 'admin-users', component: AdminUsersView, meta: { admin: true } },
        { path: 'admin/departments', name: 'admin-departments', component: AdminDepartmentsView, meta: { admin: true } },
        { path: 'workspace', name: 'workspace', component: WorkspaceHomeView },
        { path: 'workspace/services', name: 'department-work', component: DepartmentWorkView },
        { path: 'change-password', name: 'change-password', component: PasswordChangeView },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach((to) => {
  const principal = authState.principal
  if (to.name === 'login') {
    if (!principal) return true
    return principal.role === 'admin' ? { name: 'admin-home' } : { name: 'workspace' }
  }
  if (!principal) return { name: 'login' }
  if (principal.must_change_password && to.name !== 'change-password') {
    return { name: 'change-password' }
  }
  if (to.name === 'change-password' && !principal.must_change_password) return true
  if (to.meta.admin && principal.role !== 'admin') return { name: 'workspace' }
  if (principal.role === 'admin' && (to.name === 'workspace' || to.name === 'home')) return { name: 'admin-home' }
  return true
})

export default router
