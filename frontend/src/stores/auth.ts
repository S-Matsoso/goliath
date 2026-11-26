import { reactive } from 'vue'
import { apiRequest, setAccessToken } from '../services/api'
import type { Principal, Session } from '../types'

interface AuthState {
  token: string | null
  principal: Principal | null
}

export const authState = reactive<AuthState>({
  token: null,
  principal: null,
})

export function acceptSession(session: Session) {
  authState.token = session.token
  authState.principal = session.principal
  setAccessToken(session.token)
}

export async function signOut() {
  try {
    if (authState.token) await apiRequest('/auth/logout', { method: 'POST' })
  } finally {
    clearSession()
  }
}

export function clearSession() {
  authState.token = null
  authState.principal = null
  setAccessToken(null)
}
