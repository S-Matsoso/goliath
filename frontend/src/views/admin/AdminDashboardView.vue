<template>
  <div class="view">
    <div class="page-heading page-heading--spread">
      <div>
        <p class="eyebrow">ADMINISTRATION / SYSTEM HEALTH</p>
        <h1>System overview</h1>
        <p class="muted">Live health, runtime and database metrics for this registry node.</p>
      </div>
      <div class="heading-actions">
        <span v-if="metrics" class="last-updated">Updated {{ updatedAt }}</span>
        <button class="button button--secondary" :disabled="loading" @click="loadMetrics">
          <span aria-hidden="true">↻</span> {{ loading ? 'Refreshing…' : 'Refresh' }}
        </button>
      </div>
    </div>

    <div v-if="error" class="notice notice--error" role="alert">
      <strong>Metrics unavailable</strong>
      <span>{{ error }}</span>
      <button class="button button--small button--secondary" @click="loadMetrics">Try again</button>
    </div>

    <section v-if="health || metrics" class="health-banner" :class="systemHealthy ? 'health-banner--ok' : 'health-banner--degraded'">
      <div class="health-banner__icon">{{ systemHealthy ? '✓' : '!' }}</div>
      <div>
        <strong>{{ systemHealthy ? 'All systems operational' : 'System health needs attention' }}</strong>
        <p>{{ health?.service ?? metrics?.service }} · Database {{ databaseStatus }}</p>
      </div>
      <StatusBadge :label="systemHealthy ? 'HEALTHY' : 'DEGRADED'" :tone="systemHealthy ? 'success' : 'danger'" />
    </section>

    <template v-if="metrics">
      <section class="metric-grid">
        <MetricCard label="Service uptime" :value="formatUptime(metrics.uptime.uptime_seconds)" detail="Since this node started" icon="◷" />
        <MetricCard label="Requests served" :value="formatNumber(metrics.uptime.requests_total)" detail="Since process start" icon="↗" />
        <MetricCard label="Average response" :value="`${metrics.uptime.average_latency_ms.toFixed(1)} ms`" detail="Across recorded requests" icon="⌁" />
        <MetricCard label="Server errors" :value="formatNumber(metrics.uptime.server_errors_total)" detail="HTTP 5xx responses" icon="!" />
      </section>

      <div class="dashboard-grid">
        <BaseCard title="Application runtime" eyebrow="PROCESS METRICS" description="Current Go process memory and runtime activity.">
          <div class="detail-grid">
            <div class="detail-item"><span>Goroutines</span><strong>{{ formatNumber(metrics.uptime.goroutines) }}</strong></div>
            <div class="detail-item"><span>CPU cores / Go workers</span><strong>{{ metrics.uptime.cpu_cores }} / {{ metrics.uptime.gomaxprocs }}</strong></div>
            <div class="detail-item"><span>Process CPU time</span><strong>{{ metrics.uptime.cpu_seconds_total.toFixed(1) }} s</strong></div>
            <div class="detail-item"><span>CPU time (user)</span><strong>{{ metrics.uptime.cpu_seconds_user.toFixed(1) }} s</strong></div>
            <div class="detail-item"><span>Heap in use</span><strong>{{ formatBytes(metrics.uptime.heap_in_use_bytes) }}</strong></div>
            <div class="detail-item"><span>Heap allocated</span><strong>{{ formatBytes(metrics.uptime.heap_alloc_bytes) }}</strong></div>
            <div class="detail-item"><span>Total allocated</span><strong>{{ formatBytes(metrics.uptime.total_alloc_bytes) }}</strong></div>
            <div class="detail-item"><span>Garbage collections</span><strong>{{ formatNumber(metrics.uptime.gc_cycles) }}</strong></div>
            <div class="detail-item"><span>Last health check</span><strong>{{ checkedAt }}</strong></div>
          </div>
        </BaseCard>

        <BaseCard title="Database connections" eyebrow="MYSQL POOL" description="Connection availability and wait pressure.">
          <div class="db-pool">
            <div class="db-pool__summary">
              <StatusBadge :label="metrics.database.status.toUpperCase()" :tone="metrics.database.status === 'healthy' ? 'success' : 'danger'" />
              <strong>{{ metrics.database.in_use_connections }} <span>/ {{ metrics.database.max_connections }}</span></strong>
              <small>connections in use</small>
            </div>
            <div class="pool-bar" role="progressbar" :aria-valuenow="poolPercent" aria-valuemin="0" aria-valuemax="100">
              <div class="pool-bar__fill" :style="{ width: `${poolPercent}%` }" />
            </div>
            <div class="detail-grid detail-grid--compact">
              <div class="detail-item"><span>Open</span><strong>{{ metrics.database.open_connections }}</strong></div>
              <div class="detail-item"><span>Idle</span><strong>{{ metrics.database.idle_connections }}</strong></div>
              <div class="detail-item"><span>Waits</span><strong>{{ formatNumber(metrics.database.wait_count) }}</strong></div>
              <div class="detail-item"><span>Wait time</span><strong>{{ metrics.database.wait_duration_ms }} ms</strong></div>
            </div>
          </div>
        </BaseCard>

        <BaseCard title="HTTP response codes" eyebrow="TRAFFIC" description="Requests observed during this process lifetime.">
          <div v-if="responseCodes.length" class="status-list">
            <div v-for="item in responseCodes" :key="item.code" class="status-row">
              <StatusBadge :label="item.code" :tone="statusTone(item.code)" />
              <div class="status-row__bar"><span :style="{ width: `${statusPercent(item.count)}%` }" /></div>
              <strong>{{ formatNumber(item.count) }}</strong>
            </div>
          </div>
          <p v-else class="empty-state">No completed HTTP requests recorded yet.</p>
        </BaseCard>

        <BaseCard title="Administration" eyebrow="QUICK LINKS" description="Manage access and department workspaces.">
          <div class="quick-link-list">
            <RouterLink :to="{ name: 'admin-users' }" class="quick-link">
              <span class="quick-link__icon">♙</span>
              <span><strong>Manage user accounts</strong><small>Create access and issue temporary credentials</small></span>
              <span aria-hidden="true">→</span>
            </RouterLink>
            <RouterLink :to="{ name: 'admin-departments' }" class="quick-link">
              <span class="quick-link__icon">▦</span>
              <span><strong>Manage departments</strong><small>Maintain active department workspaces</small></span>
              <span aria-hidden="true">→</span>
            </RouterLink>
          </div>
        </BaseCard>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import BaseCard from '../../components/BaseCard.vue'
import MetricCard from '../../components/MetricCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { ApiError, apiRequest } from '../../services/api'
import type { HealthStatus, SystemMetrics } from '../../types'

const metrics = ref<SystemMetrics | null>(null)
const health = ref<HealthStatus | null>(null)
const loading = ref(false)
const error = ref('')
const updated = ref<Date | null>(null)
let refreshTimer: ReturnType<typeof setInterval> | undefined
const updatedAt = computed(() => updated.value?.toLocaleTimeString() ?? '')
const checkedAt = computed(() => metrics.value ? new Date(metrics.value.checked_at).toLocaleTimeString() : '—')
const databaseStatus = computed(() => health.value?.database ?? metrics.value?.database.status ?? 'unknown')
const systemHealthy = computed(() =>
  (health.value?.status === 'ok' || health.value?.status === 'healthy' || metrics.value?.status === 'healthy') &&
  databaseStatus.value === 'healthy',
)
const responseCodes = computed(() => Object.entries(metrics.value?.uptime.status_codes ?? {})
  .map(([code, count]) => ({ code, count }))
  .sort((left, right) => left.code.localeCompare(right.code)))
const poolPercent = computed(() => {
  const maximum = metrics.value?.database.max_connections ?? 0
  if (!maximum) return 0
  return Math.min(100, Math.round((metrics.value!.database.in_use_connections / maximum) * 100))
})
const highestStatusCount = computed(() => Math.max(1, ...responseCodes.value.map((item) => item.count)))

async function loadMetrics() {
  loading.value = true
  error.value = ''
  const [healthResult, metricsResult] = await Promise.allSettled([
    apiRequest<HealthStatus>('/health'),
    apiRequest<SystemMetrics>('/admin/metrics'),
  ])
  health.value = healthResult.status === 'fulfilled' ? healthResult.value : null
  if (metricsResult.status === 'fulfilled') {
    metrics.value = metricsResult.value
    updated.value = new Date()
  } else {
    const cause = metricsResult.reason
    error.value = cause instanceof ApiError ? cause.message : 'The health service could not be reached.'
  }
  loading.value = false
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let amount = value / 1024
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024
    unit++
  }
  return `${amount.toFixed(1)} ${units[unit]}`
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days) return `${days}d ${hours}h`
  if (hours) return `${hours}h ${minutes}m`
  return `${minutes}m ${Math.floor(seconds % 60)}s`
}

function statusPercent(count: number) {
  return Math.max(3, (count / highestStatusCount.value) * 100)
}

function statusTone(code: string): 'success' | 'warning' | 'danger' {
  if (code.startsWith('2')) return 'success'
  if (code.startsWith('4')) return 'warning'
  return 'danger'
}

onMounted(() => {
  void loadMetrics()
  refreshTimer = setInterval(() => void loadMetrics(), 30_000)
})

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>
