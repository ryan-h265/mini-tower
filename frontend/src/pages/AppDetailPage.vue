<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { CreateRunRequest, RunResponse } from '../api/types'
import { useToast } from '../composables/useToast'
import { formatAbsoluteTimestamp, formatRelativeTimestamp } from '../utils/time'
import CreateRunModal from '../components/apps/CreateRunModal.vue'
import FileDropZone from '../components/shared/FileDropZone.vue'
import EmptyState from '../components/shared/EmptyState.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import LoadingSpinner from '../components/shared/LoadingSpinner.vue'
import StatusBadge from '../components/shared/StatusBadge.vue'

type AppTab = 'overview' | 'runs' | 'versions'

const route = useRoute()
const router = useRouter()
const queryClient = useQueryClient()
const toast = useToast()

const appSlug = computed(() => String(route.params.slug ?? ''))
const activeTab = ref<AppTab>('overview')
const isCreateRunModalOpen = ref(false)
const createRunError = ref('')
const uploadError = ref('')
const artifact = ref<File | null>(null)

watch(
  () => route.query.tab,
  (value) => {
    if (value === 'runs' || value === 'versions' || value === 'overview') {
      activeTab.value = value
      return
    }
    activeTab.value = 'overview'
  },
  { immediate: true }
)

function setTab(tab: AppTab): void {
  if (activeTab.value === tab) return
  activeTab.value = tab
  void router.replace({ query: { ...route.query, tab } })
}

const appQuery = useQuery({
  queryKey: computed(() => ['app', appSlug.value]),
  queryFn: () => apiClient.getApp(appSlug.value),
  enabled: computed(() => Boolean(appSlug.value))
})

const versionsQuery = useQuery({
  queryKey: computed(() => ['app-versions', appSlug.value]),
  queryFn: () => apiClient.listVersions(appSlug.value),
  enabled: computed(() => Boolean(appSlug.value))
})

const runsQuery = useQuery({
  queryKey: computed(() => ['app-runs', appSlug.value]),
  queryFn: () => apiClient.listRunsByApp(appSlug.value, { limit: 50 }),
  enabled: computed(() => Boolean(appSlug.value)),
  refetchInterval: 2_000
})

const app = computed(() => appQuery.data.value)
const versions = computed(() => versionsQuery.data.value?.versions ?? [])
const latestVersion = computed(() => versions.value[0])
const backendRuns = computed(() => runsQuery.data.value?.runs ?? [])
const hasAnyError = computed(() => Boolean(appQuery.error.value || versionsQuery.error.value || runsQuery.error.value))
const isOverviewLoading = computed(
  () => activeTab.value === 'overview' && !hasAnyError.value && (!appQuery.data.value || !versionsQuery.data.value || !runsQuery.data.value)
)
const isRunsLoading = computed(
  () => (activeTab.value === 'overview' || activeTab.value === 'runs') && !runsQuery.error.value && !runsQuery.data.value
)
const isVersionsLoading = computed(
  () => activeTab.value === 'versions' && !versionsQuery.error.value && !versionsQuery.data.value
)

const pendingRuns = ref<RunResponse[]>([])
const runs = computed(() => {
  const combined = [...pendingRuns.value, ...backendRuns.value]
  const seen = new Set<number>()
  return combined.filter((run) => {
    if (seen.has(run.run_id)) return false
    seen.add(run.run_id)
    return true
  })
})

// Donut chart for runs
const completedCount = computed(() => runs.value.filter(r => r.status === 'completed').length)
const donutPercent = computed(() => {
  if (runs.value.length === 0) return 100
  return Math.round((completedCount.value / runs.value.length) * 100)
})
const donutDasharray = computed(() => {
  const circ = 2 * Math.PI * 42
  const filled = (donutPercent.value / 100) * circ
  return `${filled} ${circ - filled}`
})

function absoluteTimestamp(value?: string): string {
  return formatAbsoluteTimestamp(value)
}

function relativeTimestamp(value?: string): string {
  return formatRelativeTimestamp(value)
}

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
  const sec = Math.floor((end - start) / 1000)
  return `${String(Math.floor(sec / 60)).padStart(2, '0')}:${String(sec % 60).padStart(2, '0')}`
}

function buildOptimisticRun(payload: CreateRunRequest): RunResponse {
  const nextRunNo = backendRuns.value.reduce((max, run) => Math.max(max, run.run_no), 0) + 1
  return {
    run_id: -Date.now(),
    app_id: app.value?.app_id ?? 0,
    app_slug: appSlug.value,
    run_no: nextRunNo,
    version_no: payload.version_no ?? latestVersion.value?.version_no ?? 0,
    status: 'queued',
    input: payload.input,
    priority: payload.priority ?? 0,
    max_retries: payload.max_retries ?? 0,
    retry_count: 0,
    cancel_requested: false,
    queued_at: new Date().toISOString()
  }
}

const createRunMutation = useMutation({
  mutationFn: async (variables: { payload: CreateRunRequest }) => apiClient.createRun(appSlug.value, variables.payload),
  onMutate: async (variables) => {
    const optimistic = buildOptimisticRun(variables.payload)
    pendingRuns.value = [optimistic, ...pendingRuns.value]
    return { optimisticRunID: optimistic.run_id }
  },
  onError: (error, _variables, context) => {
    if (context?.optimisticRunID) pendingRuns.value = pendingRuns.value.filter(r => r.run_id !== context.optimisticRunID)
    createRunError.value = error instanceof Error ? error.message : 'Failed to create run'
    toast.error(createRunError.value)
  },
  onSuccess: async (created, _variables, context) => {
    if (context?.optimisticRunID) pendingRuns.value = pendingRuns.value.filter(r => r.run_id !== context.optimisticRunID)
    isCreateRunModalOpen.value = false
    createRunError.value = ''
    toast.success(`Run #${created.run_no} created.`)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['app-runs', appSlug.value] }),
      queryClient.invalidateQueries({ queryKey: ['runs'] }),
      queryClient.invalidateQueries({ queryKey: ['recent-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['runs-summary'] })
    ])
    void router.push(`/runs/${created.run_id}`)
  }
})

const uploadVersionMutation = useMutation({
  mutationFn: async () => {
    if (!artifact.value) throw new Error('Artifact file is required.')
    return apiClient.createVersion(appSlug.value, { artifact: artifact.value })
  },
  onError: (error) => {
    uploadError.value = error instanceof Error ? error.message : 'Failed to upload version'
    toast.error(uploadError.value)
  },
  onSuccess: async (version) => {
    artifact.value = null; uploadError.value = ''
    toast.success(`Version v${version.version_no} uploaded.`)
    await queryClient.invalidateQueries({ queryKey: ['app-versions', appSlug.value] })
    setTab('versions')
  }
})

function onArtifactSelected(file: File): void { artifact.value = file }
function openCreateRunModal(): void { createRunError.value = ''; isCreateRunModalOpen.value = true }
function submitCreateRun(payload: CreateRunRequest): void { createRunError.value = ''; void createRunMutation.mutateAsync({ payload }) }
function submitVersionUpload(): void {
  uploadError.value = ''
  if (!artifact.value) {
    uploadError.value = 'Artifact file is required.'
    toast.error(uploadError.value)
    return
  }
  void uploadVersionMutation.mutateAsync()
}
</script>

<template>
  <div class="page">
    <!-- Breadcrumb + actions -->
    <header class="page-header">
      <div class="breadcrumb">
        <RouterLink to="/apps" class="crumb">Apps</RouterLink>
        <span class="crumb-sep">&rsaquo;</span>
        <h1 class="crumb-current">{{ appSlug }}</h1>
      </div>
      <div class="header-actions">
        <button type="button" class="btn btn-primary" :disabled="versions.length === 0" @click="openCreateRunModal">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
          Create Run
        </button>
      </div>
    </header>

    <!-- Badges row -->
    <div class="badges-row">
      <span v-if="latestVersion" class="chip">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 3v12"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 01-9 9"/></svg>
        V{{ latestVersion.version_no }}
      </span>
      <span v-if="runs.length" class="chip">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
        {{ runs.length }}
      </span>
      <span class="badge" :class="app?.disabled ? 'badge-muted' : 'badge-green'">
        <span class="badge-dot"/>
        {{ app?.disabled ? 'Disabled' : 'Active' }}
      </span>
      <div class="tab-row">
        <button :class="{ active: activeTab === 'overview' }" @click="setTab('overview')">Overview</button>
        <button :class="{ active: activeTab === 'runs' }" @click="setTab('runs')">Runs</button>
        <button :class="{ active: activeTab === 'versions' }" @click="setTab('versions')">Versions</button>
      </div>
    </div>

    <ErrorBanner v-if="hasAnyError" :message="(appQuery.error.value || runsQuery.error.value || versionsQuery.error.value)?.message || 'Failed to load app details'" />

    <!-- Overview tab -->
    <template v-if="activeTab === 'overview'">
      <div v-if="isOverviewLoading" class="overview-grid">
        <section class="card panel">
          <span class="skeleton-line medium"/>
          <div class="skeleton-block tall"/>
        </section>
        <section class="card panel meta-panel">
          <span class="skeleton-line medium"/>
          <div class="skeleton-list">
            <span v-for="idx in 4" :key="`meta-skeleton-${idx}`" class="skeleton-line"/>
          </div>
        </section>
      </div>
      <div v-else class="overview-grid">
        <section class="card panel">
          <div class="donut-section">
            <div class="donut-wrap">
              <svg viewBox="0 0 100 100" class="donut">
                <circle cx="50" cy="50" r="42" fill="none" stroke="var(--border-default)" stroke-width="8"/>
                <circle cx="50" cy="50" r="42" fill="none" stroke="var(--accent-green)" stroke-width="8"
                  :stroke-dasharray="donutDasharray" transform="rotate(-90 50 50)" stroke-linecap="round"/>
              </svg>
              <div class="donut-label">
                <span class="donut-pct">{{ donutPercent }}%</span>
                <span class="donut-sub">Run Stats</span>
              </div>
            </div>
            <div class="overview-stats">
              <div class="os-item"><span class="os-label">Total Runs</span><strong>{{ runs.length }}</strong></div>
              <div class="os-item"><span class="os-label">Completed</span><strong class="c-green">{{ completedCount }}</strong></div>
              <div class="os-item"><span class="os-label">Latest Version</span><strong>v{{ latestVersion?.version_no ?? '-' }}</strong></div>
            </div>
          </div>
        </section>
        <section class="card panel meta-panel">
          <h3>App Metadata</h3>
          <div class="meta-grid">
            <div class="meta-item"><span class="ml">Slug</span><span>{{ app?.slug ?? appSlug }}</span></div>
            <div class="meta-item"><span class="ml">Status</span><span>{{ app?.disabled ? 'disabled' : 'active' }}</span></div>
            <div class="meta-item"><span class="ml">Created</span><span :title="absoluteTimestamp(app?.created_at)">{{ relativeTimestamp(app?.created_at) }}</span></div>
            <div class="meta-item"><span class="ml">Updated</span><span :title="absoluteTimestamp(app?.updated_at)">{{ relativeTimestamp(app?.updated_at) }}</span></div>
          </div>
        </section>
      </div>
    </template>

    <!-- Runs content (shown on Overview and Runs tabs) -->
    <template v-if="activeTab === 'overview' || activeTab === 'runs'">
      <section :class="activeTab === 'overview' ? 'card panel overview-runs-panel' : ''">
        <h2 v-if="activeTab === 'overview'">Runs</h2>
        <div v-if="isRunsLoading" class="skeleton-runs">
          <div v-for="idx in 4" :key="`runs-skeleton-${idx}`" class="run-card card">
            <span class="skeleton-line medium"/>
            <div class="skeleton-block"/>
            <span class="skeleton-line short"/>
          </div>
        </div>
        <div v-else class="runs-list">
          <component
            v-for="run in runs"
            :key="run.run_id"
            :is="run.run_id > 0 ? 'RouterLink' : 'div'"
            v-bind="run.run_id > 0 ? { to: `/runs/${run.run_id}` } : {}"
            class="run-card card"
            :class="{ clickable: run.run_id > 0 }"
          >
            <div class="run-card-head">
              <div class="run-card-left">
                <span class="run-title">{{ run.run_id > 0 ? `Run #${run.run_no}` : `#${run.run_no} (pending)` }}</span>
                <div class="run-chips">
                  <span class="chip chip-sm">DEFAULT</span>
                  <span class="chip chip-sm">V{{ run.version_no }}</span>
                  <span class="chip chip-sm">{{ run.retry_count }}/{{ run.max_retries }}</span>
                </div>
              </div>
              <StatusBadge :status="run.status" />
            </div>
            <div class="run-card-bar">
              <div class="bar-track">
                <div class="bar-fill" :style="{ width: run.finished_at ? '100%' : run.status === 'running' ? '60%' : '30%', background: statusColor(run) }"/>
              </div>
            </div>
            <div class="run-card-times">
              <span :title="absoluteTimestamp(run.queued_at)">{{ relativeTimestamp(run.queued_at) }}</span>
              <span class="run-dur">{{ runDuration(run) }}</span>
              <span :title="absoluteTimestamp(run.finished_at)">{{ relativeTimestamp(run.finished_at) }}</span>
            </div>
          </component>
          <EmptyState v-if="runs.length === 0" title="No runs yet" message="Create the first run for this app." />
        </div>
      </section>
    </template>

    <!-- Versions tab -->
    <template v-if="activeTab === 'versions'">
      <template v-if="isVersionsLoading">
        <section class="card panel">
          <span class="skeleton-line medium"/>
          <div class="skeleton-block tall"/>
        </section>
      </template>
      <template v-else>
        <section class="card panel">
          <h2>Upload New Version</h2>
          <p class="upload-hint">Upload a tar.gz artifact containing a Towerfile at its root.</p>
          <ErrorBanner v-if="uploadError" :message="uploadError" />
          <form class="upload-grid" @submit.prevent="submitVersionUpload">
            <div class="full"><FileDropZone @file-selected="onArtifactSelected" /><p class="hint">Selected: {{ artifact?.name ?? 'None' }}</p></div>
            <div class="full actions">
              <button type="submit" class="btn btn-primary" :disabled="uploadVersionMutation.isPending.value">
                {{ uploadVersionMutation.isPending.value ? 'Uploading...' : 'Upload Version' }}
              </button>
            </div>
          </form>
        </section>
        <section class="card panel" v-if="versions.length">
          <h2>Version History</h2>
          <div class="ver-list">
            <div v-for="v in versions" :key="v.version_id" class="ver-row">
              <span class="chip chip-sm">V{{ v.version_no }}</span>
              <span class="ver-entry">{{ v.entrypoint }}</span>
              <span v-if="v.towerfile_toml" class="chip chip-sm chip-towerfile">Towerfile</span>
              <span v-else class="chip chip-sm chip-legacy">Legacy</span>
              <span class="ver-sha mono">{{ v.artifact_sha256.slice(0, 12) }}...</span>
              <span class="ver-time" :title="absoluteTimestamp(v.created_at)">{{ relativeTimestamp(v.created_at) }}</span>
            </div>
          </div>
        </section>
      </template>
    </template>

    <CreateRunModal :open="isCreateRunModalOpen" :versions="versions" :busy="createRunMutation.isPending.value" :error-message="createRunError"
      @close="isCreateRunModalOpen = false" @submit="submitCreateRun" />
  </div>
</template>

<style scoped>
.page { display: grid; gap: 1.25rem; }

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
}

.breadcrumb {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.crumb {
  color: var(--text-secondary);
  font-size: 1.25rem;
  font-weight: 600;
}

.crumb:hover { color: var(--accent-blue); }

.crumb-sep {
  color: var(--text-tertiary);
  font-size: 1.25rem;
}

.crumb-current {
  font-size: 1.25rem;
}

.header-actions { display: flex; gap: 0.5rem; }

.btn {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.45rem 0.85rem;
  border-radius: var(--radius-sm);
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid var(--border-default);
  background: transparent;
}

.btn-primary {
  background: var(--accent-blue);
  border-color: var(--accent-blue);
  color: white;
}
.btn-primary:hover { background: color-mix(in srgb, var(--accent-blue) 85%, white); box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-blue) 30%, transparent); transform: translateY(-1px); }
.btn-primary:active { transform: translateY(0); }
.btn-primary:disabled { opacity: 0.5; cursor: default; }

.badges-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.6rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-full);
  font-size: 0.72rem;
  color: var(--text-secondary);
  font-weight: 500;
}

.chip-sm { font-size: 0.68rem; padding: 0.15rem 0.5rem; }

.badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.6rem;
  border-radius: var(--radius-full);
  font-size: 0.72rem;
  font-weight: 500;
  border: 1px solid var(--border-default);
}
.badge-green { color: var(--accent-green); border-color: color-mix(in srgb, var(--accent-green) 25%, var(--border-default)); }
.badge-muted { color: var(--text-tertiary); }
.badge-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }

.tab-row {
  display: flex;
  gap: 2px;
  margin-left: auto;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 2px;
}

.tab-row button {
  padding: 0.35rem 0.75rem;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.78rem;
  cursor: pointer;
  font-weight: 500;
  transition: color var(--transition-fast), background var(--transition-fast), box-shadow var(--transition-fast);
}

.tab-row button:hover:not(.active) {
  color: var(--text-primary);
}

.tab-row button.active {
  background: var(--bg-secondary);
  color: var(--text-primary);
  box-shadow: var(--shadow-soft), 0 0 8px color-mix(in srgb, var(--accent-blue) 8%, transparent);
}

/* Overview */
.overview-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
}

.panel { padding: 1rem 1.15rem; display: grid; gap: 0.75rem; }

.donut-section {
  display: flex;
  gap: 1.5rem;
  align-items: center;
}

.donut-wrap { position: relative; width: 110px; height: 110px; flex-shrink: 0; }
.donut { width: 100%; height: 100%; }
.donut-label { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.donut-pct { font-size: 1.1rem; font-weight: 700; }
.donut-sub { font-size: 0.65rem; color: var(--text-tertiary); }

.overview-stats { display: grid; gap: 0.4rem; }
.os-item { display: flex; justify-content: space-between; gap: 1rem; }
.os-label { color: var(--text-secondary); font-size: 0.82rem; }
.c-green { color: var(--accent-green); }

.meta-panel h3 { margin: 0; }
.meta-grid { display: grid; gap: 0.35rem; }
.meta-item { display: flex; justify-content: space-between; gap: 1rem; font-size: 0.85rem; padding: 0.3rem 0; border-bottom: 1px solid var(--border-default); }
.ml { color: var(--text-secondary); }

/* Runs */
.overview-runs-panel h2 { margin: 0; }

.runs-list { display: grid; gap: 0.65rem; }

.run-card {
  padding: 0.85rem 1rem;
  display: grid;
  gap: 0.5rem;
  text-decoration: none;
  color: inherit;
  border-left: 2px solid transparent;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast), transform var(--transition-fast);
}

.run-card.clickable {
  cursor: pointer;
}

.run-card.clickable:hover {
  border-left-color: var(--accent-blue);
  box-shadow: inset 0 0 20px color-mix(in srgb, var(--accent-blue) 3%, transparent);
  transform: translateX(1px);
}

.run-card.clickable:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: 2px;
}

.run-card.clickable:hover .run-title {
  color: var(--accent-blue);
}
.run-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; }
.run-card-left { display: grid; gap: 0.3rem; }
.run-title { font-weight: 600; font-size: 0.92rem; }
.run-chips { display: flex; gap: 0.3rem; }

.run-card-bar { padding-top: 0.15rem; }
.bar-track { height: 5px; border-radius: 3px; background: var(--bg-elevated); overflow: hidden; }
.bar-fill { height: 100%; border-radius: 3px; transition: width 500ms ease; }

.run-card-times {
  display: flex;
  justify-content: space-between;
  font-size: 0.72rem;
  color: var(--text-tertiary);
  font-family: var(--font-mono);
}
.run-dur { color: var(--text-secondary); }

/* Versions */
.upload-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.65rem;
}

.field { display: grid; gap: 0.3rem; }
.field span { color: var(--text-secondary); font-size: 0.78rem; }

input, textarea {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  padding: 0.5rem 0.65rem;
}

textarea { resize: vertical; font-family: var(--font-mono); font-size: 0.82rem; }

.full { grid-column: 1 / -1; }
.actions { display: flex; justify-content: flex-end; }
.hint { margin: 0.3rem 0 0; font-size: 0.78rem; color: var(--text-tertiary); }
.mono { font-family: var(--font-mono); }

.ver-list { display: grid; gap: 0.4rem; }
.ver-row {
  display: grid;
  grid-template-columns: auto 1fr auto auto auto;
  gap: 0.75rem;
  align-items: center;
  padding: 0.55rem 0.35rem;
  border-bottom: 1px solid var(--border-default);
  font-size: 0.82rem;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast);
}

.ver-row:hover {
  background: color-mix(in srgb, var(--bg-tertiary) 50%, transparent);
}
.ver-entry { color: var(--text-secondary); }
.ver-sha { color: var(--text-tertiary); font-size: 0.75rem; }
.ver-time { color: var(--text-tertiary); font-size: 0.75rem; }
.chip-towerfile { color: var(--accent-green); border-color: color-mix(in srgb, var(--accent-green) 30%, var(--border-default)); }
.chip-legacy { color: var(--text-tertiary); }
.upload-hint { margin: 0; font-size: 0.82rem; color: var(--text-secondary); }

/* Skeletons */
.skeleton-line,
.skeleton-block {
  background: linear-gradient(90deg, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 20%, color-mix(in srgb, var(--bg-elevated) 65%, white) 45%, color-mix(in srgb, var(--bg-tertiary) 80%, transparent) 80%);
  background-size: 220% 100%;
  animation: shimmer 1.5s linear infinite;
}

.skeleton-line {
  display: block;
  height: 0.72rem;
  border-radius: 999px;
}

.skeleton-line.medium {
  width: 42%;
  height: 0.82rem;
}

.skeleton-line.short {
  width: 30%;
}

.skeleton-block {
  width: 100%;
  height: 54px;
  border-radius: var(--radius-sm);
}

.skeleton-block.tall {
  height: 128px;
}

.skeleton-list {
  display: grid;
  gap: 0.5rem;
}

.skeleton-runs {
  display: grid;
  gap: 0.65rem;
}

@media (max-width: 800px) {
  .badges-row { align-items: flex-start; }
  .tab-row { margin-left: 0; width: 100%; overflow-x: auto; }
  .overview-grid { grid-template-columns: 1fr; }
  .donut-section { flex-direction: column; align-items: flex-start; gap: 0.8rem; }
  .run-card-head { flex-direction: column; align-items: flex-start; }
  .run-chips { flex-wrap: wrap; }
  .run-card-times { flex-direction: column; align-items: flex-start; gap: 0.2rem; }
  .ver-row { grid-template-columns: 1fr; gap: 0.35rem; }
  .ver-sha { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .upload-grid { grid-template-columns: 1fr; }
  .actions { justify-content: flex-start; }
  .page-header { flex-direction: column; align-items: flex-start; }
}
</style>
