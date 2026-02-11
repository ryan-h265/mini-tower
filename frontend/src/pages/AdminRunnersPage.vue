<script setup lang="ts">
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import EmptyState from '../components/shared/EmptyState.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import LoadingSpinner from '../components/shared/LoadingSpinner.vue'

const runnersQuery = useQuery({
  queryKey: ['admin-runners'],
  queryFn: () => apiClient.listAdminRunners(),
  refetchInterval: 10_000
})

const runners = computed(() => runnersQuery.data.value?.runners ?? [])

const counts = computed(() => {
  let online = 0
  let offline = 0
  let unknown = 0

  for (const runner of runners.value) {
    const status = runner.status.toLowerCase()
    if (status === 'online') {
      online += 1
    } else if (status === 'offline') {
      offline += 1
    } else {
      unknown += 1
    }
  }

  return {
    total: runners.value.length,
    online,
    offline,
    unknown
  }
})

function formatAbsolute(value?: string): string {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function formatRelative(value?: string): string {
  if (!value) {
    return '-'
  }
  const timestamp = new Date(value).getTime()
  if (Number.isNaN(timestamp)) {
    return value
  }

  const diffMs = Date.now() - timestamp
  if (diffMs < 0) {
    return 'just now'
  }

  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) {
    return `${seconds}s ago`
  }

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }

  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}h ago`
  }

  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function statusClass(status: string): string {
  const normalized = status.toLowerCase()
  if (normalized === 'online') return 'online'
  if (normalized === 'offline') return 'offline'
  return 'unknown'
}
</script>

<template>
  <div class="page">
    <header class="page-header">
      <div class="header-left">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
          <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
          <line x1="6" y1="6" x2="6.01" y2="6" />
          <line x1="6" y1="18" x2="6.01" y2="18" />
        </svg>
        <div>
          <h1>Runners</h1>
          <p class="subtitle">Admin-only visibility into runner registration and heartbeat status.</p>
        </div>
      </div>
      <LoadingSpinner v-if="runnersQuery.isFetching.value" />
    </header>

    <!-- Stats row -->
    <section class="stats-row">
      <article class="stat-card">
        <span class="stat-label">Total</span>
        <strong class="stat-value">{{ counts.total }}</strong>
      </article>
      <article class="stat-card">
        <span class="stat-label">Online</span>
        <strong class="stat-value online-val">{{ counts.online }}</strong>
        <div class="stat-dot online-dot" />
      </article>
      <article class="stat-card">
        <span class="stat-label">Offline</span>
        <strong class="stat-value offline-val">{{ counts.offline }}</strong>
        <div class="stat-dot offline-dot" />
      </article>
      <article class="stat-card">
        <span class="stat-label">Other</span>
        <strong class="stat-value">{{ counts.unknown }}</strong>
      </article>
    </section>

    <ErrorBanner v-if="runnersQuery.error.value" :message="runnersQuery.error.value.message" />

    <section v-else-if="runners.length" class="table-card">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Environment</th>
            <th>Status</th>
            <th>Last Seen</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="runner in runners" :key="runner.runner_id">
            <td class="runner-name">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2" /><line x1="6" y1="6" x2="6.01" y2="6" /></svg>
              {{ runner.name }}
            </td>
            <td><span class="env-chip">{{ runner.environment }}</span></td>
            <td>
              <span class="status-chip" :class="statusClass(runner.status)">
                <span class="dot" />
                {{ runner.status }}
              </span>
            </td>
            <td :title="formatAbsolute(runner.last_seen_at)" class="ts">{{ formatRelative(runner.last_seen_at) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <EmptyState v-else title="No runners registered" message="Register a runner to start leasing queued runs." />
  </div>
</template>

<style scoped>
.page {
  display: grid;
  gap: 1rem;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.header-left {
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
  color: var(--text-primary);
}

.header-left svg {
  margin-top: 0.15rem;
  flex-shrink: 0;
}

.header-left h1 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
}

.subtitle {
  margin: 0.15rem 0 0;
  color: var(--text-tertiary);
  font-size: 0.82rem;
}

/* Stats */
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.6rem;
}

.stat-card {
  position: relative;
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.85rem 1rem;
  display: grid;
  gap: 0.2rem;
  overflow: hidden;
}

.stat-label {
  color: var(--text-tertiary);
  font-size: 0.72rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.stat-value {
  font-size: 1.4rem;
  font-weight: 700;
  color: var(--text-primary);
}

.online-val { color: var(--accent-green); }
.offline-val { color: var(--accent-red); }

.stat-dot {
  position: absolute;
  top: 0.75rem;
  right: 0.75rem;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.online-dot {
  background: var(--accent-green);
  box-shadow: 0 0 6px color-mix(in srgb, var(--accent-green) 50%, transparent);
}

.offline-dot {
  background: var(--accent-red);
  box-shadow: 0 0 6px color-mix(in srgb, var(--accent-red) 50%, transparent);
}

/* Table */
.table-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.table {
  width: 100%;
  border-collapse: collapse;
}

th {
  text-align: left;
  padding: 0.6rem 0.75rem;
  font-size: 0.72rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-tertiary);
}

td {
  text-align: left;
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  font-size: 0.85rem;
}

tbody tr:last-child td {
  border-bottom: none;
}

tbody tr:hover {
  background: color-mix(in srgb, var(--bg-tertiary) 50%, transparent);
}

.runner-name {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  font-weight: 500;
  color: var(--text-primary);
}

.runner-name svg {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.env-chip {
  display: inline-block;
  padding: 0.12rem 0.45rem;
  border-radius: 4px;
  background: color-mix(in srgb, var(--accent-cyan) 10%, transparent);
  color: var(--accent-cyan);
  font-size: 0.75rem;
  font-weight: 500;
  font-family: var(--font-mono);
}

.status-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.15rem 0.55rem 0.15rem 0.4rem;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 500;
  text-transform: capitalize;
}

.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

.status-chip.online {
  color: var(--accent-green);
  background: color-mix(in srgb, var(--accent-green) 12%, transparent);
}

.status-chip.offline {
  color: var(--accent-red);
  background: color-mix(in srgb, var(--accent-red) 12%, transparent);
}

.status-chip.unknown {
  color: var(--text-tertiary);
  background: color-mix(in srgb, var(--border-default) 40%, transparent);
}

.ts {
  color: var(--text-secondary);
  font-size: 0.82rem;
}

@media (max-width: 700px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
