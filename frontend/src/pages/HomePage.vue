<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { RunResponse } from '../api/types'
import StatusBadge from '../components/shared/StatusBadge.vue'

const appsQuery = useQuery({
  queryKey: ['apps'],
  queryFn: () => apiClient.listApps()
})

const summaryQuery = useQuery({
  queryKey: ['runs-summary'],
  queryFn: () => apiClient.getRunsSummary()
})

const recentRunsQuery = useQuery({
  queryKey: ['recent-runs'],
  queryFn: () => apiClient.listRunsByTeam({ limit: 50 }),
  refetchInterval: 2_000
})

const appsCount = computed(() => appsQuery.data.value?.apps.length ?? 0)
const activeRuns = computed(() => summaryQuery.data.value?.active_runs ?? 0)
const queuedRuns = computed(() => summaryQuery.data.value?.queued_runs ?? 0)
const totalRuns = computed(() => summaryQuery.data.value?.total_runs ?? 0)
const terminalRuns = computed(() => summaryQuery.data.value?.terminal_runs ?? 0)
const recentRuns = computed(() => (recentRunsQuery.data.value?.runs ?? []).slice(0, 5))

const healthyApps = computed(() => {
  const apps = appsQuery.data.value?.apps ?? []
  return apps.filter(a => !a.disabled).length
})

const disabledApps = computed(() => {
  const apps = appsQuery.data.value?.apps ?? []
  return apps.filter(a => a.disabled).length
})

const isStatsLoading = computed(
  () => !appsQuery.error.value && !summaryQuery.error.value && (!appsQuery.data.value || !summaryQuery.data.value)
)
const isMainPanelsLoading = computed(
  () => !appsQuery.error.value && !summaryQuery.error.value && !recentRunsQuery.error.value
    && (!appsQuery.data.value || !summaryQuery.data.value || !recentRunsQuery.data.value)
)
const isRecentListLoading = computed(() => !recentRunsQuery.error.value && !recentRunsQuery.data.value)

const donutPercent = computed(() => {
  if (totalRuns.value === 0) return 0
  return Math.round((terminalRuns.value / totalRuns.value) * 100)
})

const donutDasharray = computed(() => {
  const circumference = 2 * Math.PI * 42
  const filled = (donutPercent.value / 100) * circumference
  return `${filled} ${circumference - filled}`
})

// Aggregate recent runs into daily bars for the past 7 days
const dailyBars = computed(() => {
  const runs = recentRunsQuery.data.value?.runs ?? []
  // Build a map of date string -> { succeeded, failed }
  const buckets = new Map<string, { succeeded: number; failed: number }>()
  const now = new Date()
  for (let d = 6; d >= 0; d--) {
    const date = new Date(now)
    date.setDate(date.getDate() - d)
    buckets.set(date.toISOString().slice(0, 10), { succeeded: 0, failed: 0 })
  }

  for (const run of runs) {
    const ts = run.finished_at ?? run.queued_at
    if (!ts) continue
    const dateKey = new Date(ts).toISOString().slice(0, 10)
    const bucket = buckets.get(dateKey)
    if (!bucket) continue
    if (run.status === 'failed' || run.status === 'dead') {
      bucket.failed++
    } else {
      bucket.succeeded++
    }
  }

  const maxTotal = Math.max(1, ...Array.from(buckets.values()).map(b => b.succeeded + b.failed))
  return Array.from(buckets.entries()).map(([date, b]) => {
    const total = b.succeeded + b.failed
    return {
      date,
      total,
      succeeded: b.succeeded,
      failed: b.failed,
      okPct: (b.succeeded / maxTotal) * 100,
      failPct: (b.failed / maxTotal) * 100,
    }
  })
})

function statusColor(run: RunResponse): string {
  switch (run.status) {
    case 'completed': return 'var(--accent-green)'
    case 'failed': case 'dead': return 'var(--accent-red)'
    case 'cancelled': case 'cancelling': return 'var(--accent-yellow)'
    case 'running': case 'leased': return 'var(--accent-blue)'
    default: return 'var(--text-tertiary)'
  }
}

function runDuration(run: RunResponse): string {
  const start = run.started_at ? new Date(run.started_at).getTime() : 0
  const end = run.finished_at ? new Date(run.finished_at).getTime() : Date.now()
  if (!start) return '--:--'
  const diffSec = Math.floor((end - start) / 1000)
  const mins = String(Math.floor(diffSec / 60)).padStart(2, '0')
  const secs = String(diffSec % 60).padStart(2, '0')
  return `${mins}:${secs}`
}

const visible = ref(false)
onMounted(() => {
  requestAnimationFrame(() => { visible.value = true })
})
</script>

<template>
  <div class="home" :class="{ visible }">
    <!-- Page header -->
    <div class="page-header">
      <div class="page-icon">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
      </div>
      <div>
        <h1>Home</h1>
        <p class="subtitle">Overview of your apps</p>
      </div>
    </div>

    <!-- Stat cards -->
    <div v-if="isStatsLoading" class="stat-row">
      <article v-for="idx in 4" :key="`stat-skeleton-${idx}`" class="stat-card stat-skeleton">
        <div class="stat-icon skeleton-box"/>
        <span class="skeleton-line short"/>
        <span class="skeleton-line tiny"/>
      </article>
    </div>
    <div v-else class="stat-row">
      <RouterLink to="/apps" class="stat-card">
        <div class="stat-icon blue">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
        </div>
        <span class="stat-label">All Apps</span>
        <span class="stat-value">{{ appsCount }}</span>
      </RouterLink>
      <RouterLink :to="{ path: '/apps', query: { status: 'healthy' } }" class="stat-card">
        <div class="stat-icon green">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M22 11.08V12a10 10 0 11-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
        </div>
        <span class="stat-label">Healthy Apps</span>
        <span class="stat-value">{{ healthyApps }}</span>
      </RouterLink>
      <RouterLink :to="{ path: '/runs', query: { status: 'running' } }" class="stat-card accent">
        <div class="stat-icon cyan">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        </div>
        <span class="stat-label">Running Apps</span>
        <span class="stat-value">{{ activeRuns }}</span>
      </RouterLink>
      <RouterLink :to="{ path: '/apps', query: { status: 'disabled' } }" class="stat-card">
        <div class="stat-icon muted">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>
        </div>
        <span class="stat-label">Disabled Apps</span>
        <span class="stat-value">{{ disabledApps }}</span>
      </RouterLink>
    </div>

    <!-- Main grid: chart + issues -->
    <div v-if="isMainPanelsLoading" class="main-grid">
      <section class="card panel">
        <div class="skeleton-line medium"/>
        <div class="skeleton-chart"/>
      </section>
      <section class="card panel">
        <div class="skeleton-line medium"/>
        <div class="skeleton-list">
          <div v-for="idx in 4" :key="`issue-skeleton-${idx}`" class="skeleton-line"/>
        </div>
      </section>
    </div>
    <div v-else class="main-grid">
      <!-- Recent Runs panel with donut -->
      <section class="card panel runs-panel">
        <header class="panel-head">
          <div class="panel-head-left">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
            <h2>Recent Runs</h2>
          </div>
          <span class="panel-meta">Past 7 days</span>
        </header>

        <div class="runs-body">
          <!-- Donut chart -->
          <div class="donut-wrap">
            <svg viewBox="0 0 100 100" class="donut">
              <circle cx="50" cy="50" r="42" fill="none" stroke="var(--border-default)" stroke-width="8"/>
              <circle cx="50" cy="50" r="42" fill="none" stroke="url(#donut-grad)" stroke-width="8"
                :stroke-dasharray="donutDasharray" stroke-dashoffset="0"
                transform="rotate(-90 50 50)" stroke-linecap="round" class="donut-arc"/>
              <defs>
                <linearGradient id="donut-grad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stop-color="var(--accent-green)"/>
                  <stop offset="100%" stop-color="var(--accent-cyan)"/>
                </linearGradient>
              </defs>
            </svg>
            <div class="donut-label">
              <span class="donut-pct">{{ donutPercent }}%</span>
              <span class="donut-sub">Run Stats</span>
            </div>
          </div>

          <!-- Bar chart from real run data -->
          <div class="chart-bars">
            <template v-if="dailyBars.length > 0">
              <div v-for="bar in dailyBars" :key="bar.date" class="bar-col" :title="`${bar.date}: ${bar.total} run${bar.total === 1 ? '' : 's'}`">
                <div class="bar bar-fail" v-if="bar.failed > 0" :style="{ height: `${bar.failPct}%` }"/>
                <div class="bar bar-ok" v-if="bar.succeeded > 0" :style="{ height: `${bar.okPct}%` }"/>
              </div>
            </template>
            <div v-else class="chart-empty">No runs in the past 7 days</div>
          </div>
        </div>
      </section>

      <!-- Apps with Issues -->
      <section class="card panel issues-panel">
        <header class="panel-head">
          <div class="panel-head-left">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            <h2>Apps with Issues</h2>
          </div>
          <span class="issues-count">{{ disabledApps }}</span>
        </header>
        <div class="issues-body">
          <div v-if="disabledApps === 0" class="issues-empty">
            <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="var(--text-tertiary)" stroke-width="1.5" stroke-linecap="round"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="M9 12l2 2 4-4"/></svg>
            <span>All apps are healthy!</span>
          </div>
          <div v-else class="issues-list">
            <RouterLink
              v-for="app in (appsQuery.data.value?.apps ?? []).filter(a => a.disabled)"
              :key="app.app_id"
              :to="`/apps/${app.slug}`"
              class="issue-row"
            >
              <span class="issue-dot"/>
              <span class="issue-name">{{ app.slug }}</span>
            </RouterLink>
          </div>
        </div>
      </section>
    </div>

    <!-- Recent runs list -->
    <section class="card panel">
      <div class="run-list">
        <template v-if="isRecentListLoading">
          <div v-for="idx in 4" :key="`run-skeleton-${idx}`" class="run-row run-row-skeleton">
            <div class="run-meta">
              <span class="skeleton-line short"/>
              <span class="skeleton-line tiny"/>
            </div>
            <div class="run-bar-row">
              <div class="run-bar-track">
                <div class="run-bar-fill skeleton-fill"/>
              </div>
              <span class="skeleton-line tiny"/>
            </div>
          </div>
        </template>
        <template v-else>
          <RouterLink
            v-for="run in recentRuns"
            :key="run.run_id"
            class="run-row"
            :to="`/runs/${run.run_id}`"
          >
            <div class="run-meta">
              <span v-if="run.app_slug" class="run-app">{{ run.app_slug }}</span>
              <span v-if="run.app_slug" class="run-sep">&rsaquo;</span>
              <span class="run-num">Run #{{ run.run_no }}</span>
              <StatusBadge :status="run.status" />
            </div>
            <div class="run-bar-row">
              <div class="run-bar-track">
                <div class="run-bar-fill" :style="{ width: run.finished_at || run.status === 'completed' || run.status === 'failed' ? '100%' : '60%', background: statusColor(run) }"/>
              </div>
              <span class="run-dur">{{ runDuration(run) }}</span>
            </div>
          </RouterLink>
          <p v-if="recentRuns.length === 0" class="empty-text">No runs yet.</p>
        </template>
      </div>
      <div class="panel-foot">
        <RouterLink to="/runs" class="view-all">View all &rsaquo;</RouterLink>
      </div>
    </section>

  </div>
</template>

<style scoped>
.home {
  display: grid;
  gap: 1.25rem;
  opacity: 0;
  transform: translateY(8px);
}

.home.visible {
  animation: fadeInUp 500ms ease forwards;
}

/* Page header */
.page-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.page-icon {
  width: 44px;
  height: 44px;
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--accent-blue) 10%, var(--bg-tertiary));
  display: grid;
  place-items: center;
  color: var(--accent-blue);
  box-shadow: 0 0 16px color-mix(in srgb, var(--accent-blue) 8%, transparent);
}

.subtitle {
  margin: 0.1rem 0 0;
  color: var(--text-secondary);
  font-size: 0.82rem;
}

/* Stat row */
.stat-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.75rem;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.85rem 1rem;
  background: var(--gradient-surface);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  text-decoration: none;
  color: inherit;
  transition: border-color var(--transition-base), box-shadow var(--transition-base), transform var(--transition-base);
}

.stat-card:hover {
  border-color: var(--border-strong);
  transform: translateY(-1px);
  box-shadow: var(--shadow-card), var(--shadow-glow-blue);
}

.stat-card:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: 2px;
}

.stat-card.accent {
  border-color: color-mix(in srgb, var(--accent-cyan) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-cyan) 4%, var(--bg-secondary));
}

.stat-card.accent:hover {
  box-shadow: var(--shadow-card), var(--shadow-glow-cyan);
}

.stat-icon {
  width: 36px;
  height: 36px;
  border-radius: var(--radius-sm);
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.stat-icon.blue { background: color-mix(in srgb, var(--accent-blue) 14%, transparent); color: var(--accent-blue); box-shadow: 0 0 10px color-mix(in srgb, var(--accent-blue) 10%, transparent); }
.stat-icon.green { background: color-mix(in srgb, var(--accent-green) 14%, transparent); color: var(--accent-green); box-shadow: 0 0 10px color-mix(in srgb, var(--accent-green) 10%, transparent); }
.stat-icon.cyan { background: color-mix(in srgb, var(--accent-cyan) 14%, transparent); color: var(--accent-cyan); box-shadow: 0 0 10px color-mix(in srgb, var(--accent-cyan) 10%, transparent); }
.stat-icon.muted { background: var(--bg-tertiary); color: var(--text-tertiary); }

.stat-label {
  color: var(--text-secondary);
  font-size: 0.8rem;
  flex: 1;
}

.stat-value {
  font-size: 1.35rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
}

/* Skeletons */
.stat-skeleton {
  pointer-events: none;
}

.skeleton-box,
.skeleton-line,
.skeleton-fill,
.skeleton-chart {
  background: linear-gradient(90deg, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 20%, color-mix(in srgb, var(--bg-elevated) 65%, white) 45%, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 80%);
  background-size: 220% 100%;
  animation: shimmer 1.5s linear infinite;
}

.skeleton-box {
  width: 36px;
  height: 36px;
  border-radius: var(--radius-sm);
}

.skeleton-line {
  display: block;
  height: 0.72rem;
  border-radius: 999px;
}

.skeleton-line.short { width: 55%; }
.skeleton-line.medium { width: 38%; height: 0.85rem; }
.skeleton-line.tiny { width: 26%; }

.skeleton-chart {
  width: 100%;
  height: 140px;
  border-radius: var(--radius-sm);
}

.skeleton-list {
  display: grid;
  gap: 0.45rem;
}

/* Main grid */
.main-grid {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 0.75rem;
}

.panel {
  padding: 1rem 1.15rem;
  display: grid;
  gap: 0.85rem;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.panel-head-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--text-primary);
}

.panel-head-left svg {
  color: var(--text-secondary);
}

.panel-meta {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

/* Runs panel */
.runs-body {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: 1.25rem;
  align-items: center;
}

.donut-wrap {
  position: relative;
  width: 120px;
  height: 120px;
}

.donut {
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 0 8px color-mix(in srgb, var(--accent-green) 15%, transparent));
}

.donut-arc {
  animation: draw-in 1.2s ease-out forwards;
}

.donut-label {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.donut-pct {
  font-size: 1.15rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.donut-sub {
  font-size: 0.68rem;
  color: var(--text-tertiary);
}

.chart-bars {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  height: 80px;
}

.bar-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  height: 100%;
  cursor: default;
  transition: opacity var(--transition-fast);
}

.chart-bars:hover .bar-col {
  opacity: 0.5;
}

.chart-bars:hover .bar-col:hover {
  opacity: 1;
}

.bar-col:hover .bar {
  filter: brightness(1.2);
}

.bar {
  width: 100%;
  min-height: 0;
  transition: height 600ms ease, filter var(--transition-fast);
}

.bar-ok {
  background: var(--accent-green);
  border-radius: 4px 4px 0 0;
}

.bar-fail {
  background: var(--accent-red);
  border-radius: 4px 4px 0 0;
}

.bar-ok + .bar-fail,
.bar-fail + .bar-ok {
  border-radius: 0;
}

.bar-col .bar:first-child {
  border-radius: 3px 3px 0 0;
}

.chart-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  color: var(--text-tertiary);
  font-size: 0.8rem;
}

/* Issues panel */
.issues-count {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--accent-green);
}

.issues-body {
  display: grid;
  place-items: center;
  min-height: 80px;
}

.issues-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  color: var(--text-tertiary);
  font-size: 0.85rem;
}

.issues-list {
  display: grid;
  gap: 0.4rem;
  width: 100%;
}

.issue-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.45rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  transition: border-color var(--transition-fast), background var(--transition-fast), color var(--transition-fast);
}

.issue-row:hover {
  border-color: var(--border-default);
  background: color-mix(in srgb, var(--bg-tertiary) 70%, transparent);
  color: var(--text-primary);
}

.issue-row:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: 2px;
}

.issue-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-red);
}

.issue-name {
  font-size: 0.84rem;
  font-weight: 500;
}

/* Run list */
.run-list {
  display: grid;
  gap: 0.65rem;
}

.run-row {
  padding: 0.7rem 0.85rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  display: grid;
  gap: 0.5rem;
  text-decoration: none;
  color: inherit;
  cursor: pointer;
  border-left: 2px solid transparent;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast), transform var(--transition-fast);
}

.run-row:hover {
  border-left-color: var(--accent-blue);
  box-shadow: inset 0 0 20px color-mix(in srgb, var(--accent-blue) 3%, transparent);
  transform: translateX(1px);
}

.run-row:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: 2px;
}

.run-row:hover .run-app,
.run-row:hover .run-num {
  color: var(--accent-blue);
}

.run-meta {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  flex-wrap: wrap;
}

.run-app {
  font-weight: 600;
  font-size: 0.88rem;
}

.run-sep {
  color: var(--text-tertiary);
}

.run-num {
  color: var(--text-secondary);
  font-size: 0.82rem;
}

.run-bar-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.run-bar-track {
  flex: 1;
  height: 5px;
  border-radius: 3px;
  background: var(--bg-elevated);
  overflow: hidden;
}

.run-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 600ms ease;
}

.run-row-skeleton {
  pointer-events: none;
}

.skeleton-fill {
  width: 70%;
}

.run-dur {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-tertiary);
  min-width: 40px;
  text-align: right;
}

.empty-text {
  margin: 0;
  color: var(--text-tertiary);
  text-align: center;
  padding: 1rem;
}

.panel-foot {
  display: flex;
  justify-content: center;
}

.view-all {
  font-size: 0.82rem;
  color: var(--accent-blue);
  padding: 0.35rem 0.75rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-full);
}

.view-all:hover {
  border-color: var(--accent-blue);
  background: color-mix(in srgb, var(--accent-blue) 6%, transparent);
  box-shadow: var(--shadow-glow-blue);
}

/* Community */
.community-card {
  max-width: 600px;
}

.community-card h3 {
  margin: 0;
}

.social-links {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.social-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-full);
  font-size: 0.8rem;
  color: var(--text-secondary);
  cursor: pointer;
  transition: border-color var(--transition-fast), color var(--transition-fast), background var(--transition-fast);
}

.social-chip:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

@media (max-width: 900px) {
  .stat-row {
    grid-template-columns: repeat(2, 1fr);
  }
  .main-grid {
    grid-template-columns: 1fr;
  }
  .runs-body {
    grid-template-columns: 100px 1fr;
  }
}

@media (max-width: 500px) {
  .stat-row {
    grid-template-columns: 1fr;
  }
}
</style>
