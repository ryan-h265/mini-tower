<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { RunStatus } from '../api/types'
import { formatAbsoluteTimestamp, formatRelativeTimestamp } from '../utils/time'
import EmptyState from '../components/shared/EmptyState.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import LoadingSpinner from '../components/shared/LoadingSpinner.vue'
import StatusBadge from '../components/shared/StatusBadge.vue'

const route = useRoute()
const router = useRouter()

const runStatusValues: RunStatus[] = ['queued', 'leased', 'running', 'cancelling', 'completed', 'failed', 'cancelled', 'dead']

function normalizeRunStatus(value: unknown): '' | RunStatus {
  if (typeof value !== 'string') return ''
  return runStatusValues.includes(value as RunStatus) ? (value as RunStatus) : ''
}

function normalizeQueryText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

const statusFilter = ref<'' | RunStatus>(normalizeRunStatus(route.query.status))
const appFilter = ref(normalizeQueryText(route.query.app))
const search = ref(normalizeQueryText(route.query.q))

const appsQuery = useQuery({
  queryKey: ['apps'],
  queryFn: () => apiClient.listApps()
})

const runsQuery = useQuery({
  queryKey: computed(() => ['runs', statusFilter.value, appFilter.value]),
  queryFn: () =>
    apiClient.listRunsByTeam({
      limit: 100,
      status: statusFilter.value || undefined,
      app: appFilter.value || undefined
    }),
  refetchInterval: 2_000
})

const apps = computed(() => appsQuery.data.value?.apps ?? [])
const runs = computed(() => {
  const searchTerm = search.value.trim().toLowerCase()
  const source = runsQuery.data.value?.runs ?? []
  if (!searchTerm) {
    return source
  }
  return source.filter((run) => {
    const appSlug = run.app_slug?.toLowerCase() ?? ''
    const runNo = String(run.run_no)
    return appSlug.includes(searchTerm) || run.status.includes(searchTerm) || runNo.includes(searchTerm)
  })
})
const isRunsLoading = computed(() => !runsQuery.error.value && !runsQuery.data.value)

const statuses: Array<{ value: '' | RunStatus; label: string }> = [{ value: '', label: 'All statuses' }, ...runStatusValues.map((value) => ({
  value,
  label: value.charAt(0).toUpperCase() + value.slice(1)
}))]

function absoluteTimestamp(value?: string): string {
  return formatAbsoluteTimestamp(value)
}

function relativeTimestamp(value?: string): string {
  return formatRelativeTimestamp(value)
}

function openRun(runID: number): void {
  void router.push(`/runs/${runID}`)
}

watch(
  () => route.query.status,
  (value) => {
    const normalized = normalizeRunStatus(value)
    if (statusFilter.value !== normalized) statusFilter.value = normalized
  }
)

watch(
  () => route.query.app,
  (value) => {
    const normalized = normalizeQueryText(value)
    if (appFilter.value !== normalized) appFilter.value = normalized
  }
)

watch(
  () => route.query.q,
  (value) => {
    const normalized = normalizeQueryText(value)
    if (search.value !== normalized) search.value = normalized
  }
)

watch(statusFilter, (value) => {
  const current = normalizeRunStatus(route.query.status)
  if (current === value) return
  const nextQuery = { ...route.query }
  if (value) nextQuery.status = value
  else delete nextQuery.status
  void router.replace({ query: nextQuery })
})

watch(appFilter, (value) => {
  const current = normalizeQueryText(route.query.app)
  if (current === value) return
  const nextQuery = { ...route.query }
  if (value) nextQuery.app = value
  else delete nextQuery.app
  void router.replace({ query: nextQuery })
})

watch(search, (value) => {
  const current = normalizeQueryText(route.query.q)
  if (current === value) return
  const nextQuery = { ...route.query }
  if (value) nextQuery.q = value
  else delete nextQuery.q
  void router.replace({ query: nextQuery })
})
</script>

<template>
  <div class="page">
    <header class="page-header">
      <div class="header-left">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
        </svg>
        <h1>Global Runs</h1>
      </div>
      <LoadingSpinner v-if="runsQuery.isFetching.value" />
    </header>

    <section class="filters">
      <div class="search-box">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" /></svg>
        <input v-model.trim="search" type="search" placeholder="Search run #, app, or status..." />
      </div>
      <select v-model="statusFilter">
        <option v-for="status in statuses" :key="status.label" :value="status.value">{{ status.label }}</option>
      </select>
      <select v-model="appFilter">
        <option value="">All apps</option>
        <option v-for="app in apps" :key="app.app_id" :value="app.slug">{{ app.slug }}</option>
      </select>
    </section>

    <ErrorBanner v-if="runsQuery.error.value" :message="runsQuery.error.value.message" />

    <section v-if="isRunsLoading" class="table-card">
      <table class="table table-skeleton" aria-hidden="true">
        <thead>
          <tr>
            <th>Run</th>
            <th>App</th>
            <th>Status</th>
            <th>Version</th>
            <th>Queued</th>
            <th>Started</th>
            <th>Finished</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="idx in 8" :key="`run-skeleton-${idx}`">
            <td><span class="skeleton-line medium"/></td>
            <td><span class="skeleton-line medium"/></td>
            <td><span class="skeleton-line short"/></td>
            <td><span class="skeleton-line tiny"/></td>
            <td><span class="skeleton-line medium"/></td>
            <td><span class="skeleton-line medium"/></td>
            <td><span class="skeleton-line medium"/></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-else-if="runs.length" class="table-card">
      <table class="table">
        <thead>
          <tr>
            <th>Run</th>
            <th>App</th>
            <th>Status</th>
            <th>Version</th>
            <th>Queued</th>
            <th>Started</th>
            <th>Finished</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="run in runs"
            :key="run.run_id"
            class="run-row"
            role="link"
            tabindex="0"
            @click="openRun(run.run_id)"
            @keydown.enter.prevent="openRun(run.run_id)"
            @keydown.space.prevent="openRun(run.run_id)"
          >
            <td>
              <span class="run-link">#{{ run.run_no }}</span>
            </td>
            <td>
              <RouterLink
                v-if="run.app_slug"
                :to="`/apps/${run.app_slug}?tab=runs`"
                class="app-link"
                @click.stop
                @keydown.enter.stop
                @keydown.space.stop
              >
                {{ run.app_slug }}
              </RouterLink>
              <span v-else class="muted">-</span>
            </td>
            <td><StatusBadge :status="run.status" /></td>
            <td><span class="version-chip">v{{ run.version_no }}</span></td>
            <td class="ts"><span :title="absoluteTimestamp(run.queued_at)">{{ relativeTimestamp(run.queued_at) }}</span></td>
            <td class="ts"><span :title="absoluteTimestamp(run.started_at)">{{ relativeTimestamp(run.started_at) }}</span></td>
            <td class="ts"><span :title="absoluteTimestamp(run.finished_at)">{{ relativeTimestamp(run.finished_at) }}</span></td>
          </tr>
        </tbody>
      </table>
    </section>

    <EmptyState v-else title="No runs found" message="Adjust the filters or create a new run from an app page." />
  </div>
</template>

<style scoped>
.page {
  display: grid;
  gap: 1rem;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--text-primary);
}

.header-left h1 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
}

.filters {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 0.5rem;
}

.search-box {
  position: relative;
  display: flex;
  align-items: center;
}

.search-box svg {
  position: absolute;
  left: 0.7rem;
  color: var(--text-tertiary);
  pointer-events: none;
}

.search-box input {
  width: 100%;
  padding-left: 2.2rem;
}

.search-box:focus-within svg {
  color: var(--accent-blue);
  transition: color var(--transition-fast);
}

input, select {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
  color: var(--text-primary);
  padding: 0.5rem 0.7rem;
  font-size: 0.85rem;
  transition: border-color var(--transition-fast);
}

input:focus, select:focus {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
  outline: none;
}

select {
  cursor: pointer;
  min-width: 140px;
}

.table-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow-x: auto;
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
  padding: 0.55rem 0.75rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  font-size: 0.85rem;
}

tbody tr:last-child td {
  border-bottom: none;
}

tbody tr {
  transition: background var(--transition-fast);
}

.run-row {
  cursor: pointer;
}

.run-row:hover {
  background: color-mix(in srgb, var(--bg-tertiary) 60%, transparent);
}

.run-row:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: -2px;
}

.run-row:hover .run-link {
  text-decoration: underline;
}

.run-link {
  color: var(--accent-blue);
  text-decoration: none;
  font-weight: 600;
  font-family: var(--font-mono);
  font-size: 0.82rem;
}

.run-link:hover {
  text-decoration: underline;
}

.app-link {
  color: var(--text-primary);
  text-decoration: none;
  font-weight: 500;
}

.app-link:hover {
  color: var(--accent-blue);
}

.muted {
  color: var(--text-tertiary);
}

.version-chip {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  background: color-mix(in srgb, var(--accent-purple) 12%, transparent);
  color: var(--accent-purple);
  font-size: 0.75rem;
  font-weight: 600;
  font-family: var(--font-mono);
}

.ts {
  color: var(--text-secondary);
  font-size: 0.8rem;
  white-space: nowrap;
}

.table-skeleton .skeleton-line {
  display: block;
  height: 0.68rem;
  border-radius: 999px;
  background: linear-gradient(90deg, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 20%, color-mix(in srgb, var(--bg-elevated) 65%, white) 45%, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 80%);
  background-size: 220% 100%;
  animation: shimmer 1.5s linear infinite;
}

.table-skeleton .skeleton-line.medium { width: 66%; }
.table-skeleton .skeleton-line.short { width: 44%; }
.table-skeleton .skeleton-line.tiny { width: 35%; }

@media (max-width: 900px) {
  .filters {
    grid-template-columns: 1fr;
  }

  .table th:nth-child(6),
  .table th:nth-child(7),
  .table td:nth-child(6),
  .table td:nth-child(7) {
    display: none;
  }
}

@media (max-width: 700px) {
  .table th:nth-child(5),
  .table td:nth-child(5) {
    display: none;
  }

  th, td {
    padding: 0.48rem 0.55rem;
  }
}
</style>
