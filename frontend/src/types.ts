export interface Principal {
  official_id: number
  account_id: number
  username: string
  display_name: string
  department: string
  role: string
  must_change_password: boolean
}

export interface Session {
  token: string
  expires_at: string
  principal: Principal
  must_change_password: boolean
}

export interface ApiErrorPayload {
  error?: {
    code?: string
    message?: string
  }
}

export interface Department {
  code: string
  name: string
  active: boolean
  member_count: number
}

export interface ManagedUser {
  id: number
  username: string
  display_name: string
  department: string
  department_name: string
  role: string
  active: boolean
  must_change_password: boolean
  created_at: string
  last_login_at?: string
}

export interface SystemMetrics {
  service: string
  status: 'healthy' | 'degraded'
  checked_at: string
  uptime: {
    uptime_seconds: number
    requests_total: number
    server_errors_total: number
    average_latency_ms: number
    status_codes: Record<string, number>
    goroutines: number
    cpu_cores: number
    gomaxprocs: number
    cpu_seconds_total: number
    cpu_seconds_user: number
    heap_in_use_bytes: number
    heap_alloc_bytes: number
    total_alloc_bytes: number
    gc_cycles: number
  }
  database: {
    status: 'healthy' | 'unavailable'
    open_connections: number
    in_use_connections: number
    idle_connections: number
    max_connections: number
    wait_count: number
    wait_duration_ms: number
  }
}

export interface HealthStatus {
  status: 'healthy' | 'degraded' | 'ok'
  service: string
  database?: 'healthy' | 'unavailable'
}
