<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { CreateRunRequest, RunLogEntry, RunStatus } from '../api/types'
import CreateRunModal from '../components/apps/CreateRunModal.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import StatusBadge from '../components/shared/StatusBadge.vue'

const route = useRoute()
const router = useRouter()
const queryClient = useQueryClient()
const runId = computed(() => Number(route.params.runId))

const logs = ref<RunLogEntry[]>([])
const lastSeq = ref(0)
const actionError = ref('')
const isRerunModalOpen = ref(false)
const rerunError = ref('')
const wrapLogs = ref(true)
const followLogs = ref(true)
const copied = ref(false)

const logsViewport = ref<HTMLElement | null>(null)
const statusOverride = ref<RunStatus | null>(null)
const cancelRequestedOverride = ref(false)

function isActive(status?: RunStatus): boolean {
  return status === 'queued' || status === 'leased' || status === 'running' || status === 'cancelling'
}

const runQuery = useQuery({
  queryKey: computed(() => ['run', runId.value]),
  queryFn: () => apiClient.getRun(runId.value),
  enabled: computed(() => Number.isFinite(runId.value) && runId.value > 0),
  refetchInterval: (query) => {
    const status = query.state.data?.status as RunStatus | undefined
    return isActive(status) ? 2_000 : false
  }
})

const run = computed(() => {
  const base = runQuery.data.value
  if (!base) return undefined
  return {
    ...base,
    status: statusOverride.value ?? base.status,
    cancel_requested: cancelRequestedOverride.value || base.cancel_requested
  }
})

const canCancel = computed(() => isActive(run.value?.status) && !run.value?.cancel_requested)
const logsRefetchInterval = computed(() => {
  const status = run.value?.status
  if (!status) return 2_000
  return isActive(status) ? 2_000 : false
})

const logsQuery = useQuery({
  queryKey: computed(() => ['run-logs', runId.value]),
  queryFn: async () => {
    const response = await apiClient.getRunLogs(runId.value, lastSeq.value)
    return response.logs
  },
  enabled: computed(() => Number.isFinite(runId.value) && runId.value > 0),
  refetchInterval: logsRefetchInterval
})

const versionsQuery = useQuery({
  queryKey: computed(() => ['app-versions', run.value?.app_slug ?? '']),
  queryFn: () => apiClient.listVersions(run.value?.app_slug ?? ''),
  enabled: computed(() => Boolean(run.value?.app_slug))
})

watch(() => runId.value, () => {
  logs.value = []; lastSeq.value = 0; statusOverride.value = null; cancelRequestedOverride.value = false; actionError.value = ''
})

watch(() => run.value?.status, (status, previousStatus) => {
  if (!status || !previousStatus) return
  if (!isActive(status) && isActive(previousStatus)) {
    void logsQuery.refetch()
  }
})

watch(() => logsQuery.data.value, async (entries) => {
  if (!entries || entries.length === 0) return
  const known = new Set(logs.value.map(e => e.seq))
  for (const entry of entries) {
    if (!known.has(entry.seq)) logs.value.push(entry)
    if (entry.seq > lastSeq.value) lastSeq.value = entry.seq
  }
  if (followLogs.value) {
    await nextTick()
    if (logsViewport.value) logsViewport.value.scrollTop = logsViewport.value.scrollHeight
  }
})

const cancelMutation = useMutation({
  mutationFn: () => apiClient.cancelRun(runId.value),
  onMutate: () => {
    actionError.value = ''; cancelRequestedOverride.value = true
    if (run.value?.status !== 'queued') statusOverride.value = 'cancelling'
  },
  onError: (error) => {
    statusOverride.value = null; cancelRequestedOverride.value = false
    actionError.value = error instanceof Error ? error.message : 'Failed to cancel run'
  },
  onSuccess: (updatedRun) => {
    statusOverride.value = null; cancelRequestedOverride.value = false
    queryClient.setQueryData(['run', runId.value], updatedRun)
    void queryClient.invalidateQueries({ queryKey: ['runs'] })
    void queryClient.invalidateQueries({ queryKey: ['recent-runs'] })
    void queryClient.invalidateQueries({ queryKey: ['runs-summary'] })
  }
})

const rerunMutation = useMutation({
  mutationFn: (payload: CreateRunRequest) => {
    if (!run.value?.app_slug) throw new Error('Run app slug is unavailable.')
    return apiClient.createRun(run.value.app_slug, payload)
  },
  onError: (error) => { rerunError.value = error instanceof Error ? error.message : 'Failed to create rerun' },
  onSuccess: async (created) => {
    rerunError.value = ''; isRerunModalOpen.value = false
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['runs'] }),
      queryClient.invalidateQueries({ queryKey: ['recent-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['runs-summary'] }),
      queryClient.invalidateQueries({ queryKey: ['app-runs', run.value?.app_slug ?? ''] })
    ])
    void router.push(`/runs/${created.run_id}`)
  }
})

const rerunSeed = computed(() => {
  if (!run.value) return undefined
  return { version_no: run.value.version_no, input: run.value.input ?? {}, priority: run.value.priority, max_retries: run.value.max_retries }
})
const rerunVersions = computed(() => versionsQuery.data.value?.versions ?? [])

function formatTimestamp(value?: string): string {
  if (!value) return '-'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

function runDuration(): string {
  if (!run.value?.started_at) return '--:--'
  const start = new Date(run.value.started_at).getTime()
  const end = run.value.finished_at ? new Date(run.value.finished_at).getTime() : Date.now()
  const sec = Math.floor((end - start) / 1000)
  return `${String(Math.floor(sec / 60)).padStart(2, '0')}:${String(sec % 60).padStart(2, '0')}`
}

function statusColor(): string {
  switch (run.value?.status) {
    case 'completed': return 'var(--accent-green)'
    case 'failed': case 'dead': return 'var(--accent-red)'
    case 'cancelled': case 'cancelling': return 'var(--accent-yellow)'
    case 'running': case 'leased': return 'var(--accent-blue)'
    default: return 'var(--text-tertiary)'
  }
}

function streamLabel(stream: string): string {
  return stream === 'stderr' ? 'SETUP' : 'APP'
}

function openRerunModal(): void { rerunError.value = ''; isRerunModalOpen.value = true }
function submitRerun(payload: CreateRunRequest): void { rerunError.value = ''; void rerunMutation.mutateAsync(payload) }

async function copyLogs(): Promise<void> {
  const text = logs.value.map(e => `[${e.stream}] ${e.line}`).join('\n')
  if (!text) return
  try { await navigator.clipboard.writeText(text); copied.value = true; window.setTimeout(() => { copied.value = false }, 1000) }
  catch { actionError.value = 'Clipboard copy is unavailable in this browser context.' }
}
</script>

<template>
  <div class="page">
    <!-- Breadcrumb header -->
    <header class="page-header">
      <div class="breadcrumb">
        <RouterLink to="/apps" class="crumb">Apps</RouterLink>
        <span class="crumb-sep">&rsaquo;</span>
        <RouterLink v-if="run?.app_slug" :to="`/apps/${run.app_slug}?tab=runs`" class="crumb">{{ run.app_slug }}</RouterLink>
        <span class="crumb-sep">&rsaquo;</span>
        <h1 class="crumb-current">Run #{{ run?.run_no ?? runId }}</h1>
      </div>
      <div class="header-actions">
        <span v-if="run" class="chip">DEFAULT</span>
        <span v-if="run" class="chip">V{{ run.version_no }}</span>
        <button type="button" class="btn btn-primary" :disabled="!run?.app_slug" @click="openRerunModal">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 102.13-9.36L1 10"/></svg>
          Rerun
        </button>
      </div>
    </header>

    <ErrorBanner v-if="runQuery.error.value" :message="runQuery.error.value.message" />
    <ErrorBanner v-if="actionError" :message="actionError" />

    <!-- Status bar -->
    <div v-if="run" class="status-section">
      <div class="status-bar-card card">
        <div class="status-top">
          <StatusBadge :status="run.status" />
        </div>
        <div class="progress-track">
          <div class="progress-fill" :class="{ active: isActive(run.status) }" :style="{ width: run.finished_at ? '100%' : isActive(run.status) ? '60%' : '0%', '--status-color': statusColor() }"/>
        </div>
        <div class="status-times">
          <span>{{ formatTimestamp(run.started_at) }}</span>
          <span class="dur">{{ runDuration() }}</span>
          <span>{{ formatTimestamp(run.finished_at) }}</span>
        </div>
      </div>
    </div>

    <!-- Log viewer -->
    <section class="log-section card">
      <header class="log-toolbar">
        <div class="log-toolbar-left">
          <button v-if="canCancel" type="button" class="btn btn-sm btn-warn" :disabled="cancelMutation.isPending.value" @click="cancelMutation.mutate()">
            {{ cancelMutation.isPending.value ? 'Cancelling...' : 'Cancel' }}
          </button>
        </div>
        <div class="log-toolbar-right">
          <label class="toggle"><input v-model="wrapLogs" type="checkbox" /><span>Wrap</span></label>
          <label class="toggle"><input v-model="followLogs" type="checkbox" /><span>Follow</span></label>
          <button type="button" class="btn btn-sm" @click="copyLogs">{{ copied ? 'Copied' : 'Copy' }}</button>
        </div>
      </header>

      <ErrorBanner v-if="logsQuery.error.value" :message="logsQuery.error.value.message" />

      <div ref="logsViewport" class="log-viewport" :class="{ nowrap: !wrapLogs }">
        <div v-for="entry in logs" :key="entry.seq" class="log-line">
          <span class="log-seq">{{ entry.seq }}</span>
          <span class="log-ts">{{ new Date(entry.logged_at).toLocaleString() }}</span>
          <span class="log-stream" :class="entry.stream">{{ streamLabel(entry.stream) }}</span>
          <span class="log-text">{{ entry.line }}</span>
        </div>
        <p v-if="logs.length === 0" class="log-empty">No logs yet.</p>
      </div>
    </section>

    <CreateRunModal :open="isRerunModalOpen" :versions="rerunVersions" :seed="rerunSeed" :busy="rerunMutation.isPending.value" :error-message="rerunError"
      @close="isRerunModalOpen = false" @submit="submitRerun" />
  </div>
</template>

<style scoped>
.page { display: grid; gap: 1rem; }

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
}

.breadcrumb { display: flex; align-items: center; gap: 0.4rem; }
.crumb { color: var(--text-secondary); font-size: 1.1rem; font-weight: 600; }
.crumb:hover { color: var(--accent-blue); }
.crumb-sep { color: var(--text-tertiary); }
.crumb-current { font-size: 1.1rem; }

.header-actions { display: flex; align-items: center; gap: 0.4rem; }

.chip {
  display: inline-flex; align-items: center; gap: 0.3rem;
  padding: 0.18rem 0.55rem; border: 1px solid var(--border-default);
  border-radius: var(--radius-full); font-size: 0.7rem; color: var(--text-secondary); font-weight: 500;
}

.btn {
  display: inline-flex; align-items: center; gap: 0.3rem;
  padding: 0.4rem 0.75rem; border-radius: var(--radius-sm);
  font-size: 0.82rem; font-weight: 500; cursor: pointer;
  border: 1px solid var(--border-default); background: transparent;
}
.btn-sm { padding: 0.3rem 0.6rem; font-size: 0.78rem; }
.btn-primary { background: var(--accent-blue); border-color: var(--accent-blue); color: white; }
.btn-primary:hover { background: color-mix(in srgb, var(--accent-blue) 85%, white); box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-blue) 30%, transparent); transform: translateY(-1px); }
.btn-primary:active { transform: translateY(0); }
.btn-primary:disabled { opacity: 0.5; cursor: default; }
.btn-warn { color: var(--accent-yellow); border-color: color-mix(in srgb, var(--accent-yellow) 40%, var(--border-default)); }

/* Status section */
.status-bar-card {
  padding: 0.85rem 1rem;
  display: grid;
  gap: 0.5rem;
}

.status-top { display: flex; justify-content: center; }

.progress-track {
  height: 6px;
  border-radius: 3px;
  background: var(--bg-elevated);
  position: relative;
  overflow: visible;
}

.progress-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 600ms ease;
  background: var(--status-color);
  position: relative;
}

.progress-fill::after {
  content: '';
  position: absolute;
  right: -5px;
  top: 50%;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--status-color);
  border: 2px solid var(--bg-secondary);
  transform: translateY(-50%);
  transition: opacity var(--transition-fast);
  opacity: 0;
}

.progress-fill.active::after {
  opacity: 1;
  box-shadow: 0 0 6px var(--status-color);
  animation: progress-pulse 2s ease-in-out infinite;
}

@keyframes progress-pulse {
  0%, 100% { box-shadow: 0 0 4px var(--status-color); }
  50% { box-shadow: 0 0 12px var(--status-color), 0 0 20px var(--status-color); }
}

.status-times {
  display: flex;
  justify-content: space-between;
  font-size: 0.72rem;
  font-family: var(--font-mono);
  color: var(--text-tertiary);
}

.dur { color: var(--text-secondary); font-weight: 500; }

/* Log section */
.log-section {
  display: grid;
  gap: 0;
  overflow: hidden;
}

.log-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.6rem 0.85rem;
  border-bottom: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-tertiary) 50%, var(--bg-secondary));
}

.log-toolbar-left, .log-toolbar-right {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  font-size: 0.75rem;
  color: var(--text-tertiary);
  cursor: pointer;
}

.toggle input { width: 13px; height: 13px; accent-color: var(--accent-blue); }

.log-viewport {
  max-height: 420px;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: 0.78rem;
  background: var(--bg-surface);
}

.log-viewport.nowrap .log-line {
  white-space: pre;
}

.log-line {
  display: grid;
  grid-template-columns: 32px 160px 52px minmax(0, 1fr);
  gap: 0.6rem;
  padding: 0.25rem 0.65rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
  white-space: pre-wrap;
  line-height: 1.45;
}

.log-line:hover {
  background: color-mix(in srgb, var(--accent-blue) 4%, transparent);
  border-left: 2px solid color-mix(in srgb, var(--accent-blue) 40%, transparent);
  padding-left: calc(0.65rem - 2px);
}

.log-seq {
  color: var(--text-tertiary);
  text-align: right;
}

.log-ts {
  color: var(--text-tertiary);
  font-size: 0.72rem;
}

.log-stream {
  font-size: 0.65rem;
  font-weight: 600;
  text-transform: uppercase;
  padding: 0.1rem 0.35rem;
  border-radius: 3px;
  text-align: center;
  align-self: start;
  line-height: 1.35;
}

.log-stream.stderr {
  background: color-mix(in srgb, var(--accent-yellow) 18%, transparent);
  color: var(--accent-yellow);
  border: 1px solid color-mix(in srgb, var(--accent-yellow) 20%, transparent);
}

.log-stream.stdout {
  background: color-mix(in srgb, var(--accent-green) 18%, transparent);
  color: var(--accent-green);
  border: 1px solid color-mix(in srgb, var(--accent-green) 20%, transparent);
}

.log-text {
  min-width: 0;
  color: var(--text-primary);
}

.log-empty {
  margin: 0;
  padding: 1.5rem;
  text-align: center;
  color: var(--text-tertiary);
}

@media (max-width: 700px) {
  .page-header { flex-direction: column; align-items: flex-start; }
  .log-line { grid-template-columns: 28px 52px minmax(0, 1fr); }
  .log-ts { display: none; }
}
</style>
