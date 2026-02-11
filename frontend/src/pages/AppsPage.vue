<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { AppResponse } from '../api/types'
import CreateAppModal from '../components/apps/CreateAppModal.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import LoadingSpinner from '../components/shared/LoadingSpinner.vue'

type AppStatusFilter = '' | 'healthy' | 'disabled'

function normalizeStatusFilter(value: unknown): AppStatusFilter {
  if (value === 'healthy' || value === 'disabled') return value
  return ''
}

function normalizeTextFilter(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

const route = useRoute()
const router = useRouter()
const search = ref(normalizeTextFilter(route.query.q))
const isCreateModalOpen = ref(false)
const statusFilter = computed<AppStatusFilter>(() => normalizeStatusFilter(route.query.status))

const appsQuery = useQuery({
  queryKey: ['apps'],
  queryFn: () => apiClient.listApps()
})

const apps = computed(() => appsQuery.data.value?.apps ?? [])
const filteredApps = computed(() => {
  const term = search.value.trim().toLowerCase()
  let source = apps.value
  if (statusFilter.value === 'healthy') source = source.filter(app => !app.disabled)
  else if (statusFilter.value === 'disabled') source = source.filter(app => app.disabled)
  if (!term) return source
  return source.filter((app) => {
    const desc = app.description?.toLowerCase() ?? ''
    return app.slug.toLowerCase().includes(term) || desc.includes(term)
  })
})

function handleCreated(app: AppResponse): void {
  isCreateModalOpen.value = false
  void router.push(`/apps/${app.slug}`)
}

function setStatusFilter(next: AppStatusFilter): void {
  const current = normalizeStatusFilter(route.query.status)
  if (current === next) return
  const query = { ...route.query }
  if (next) query.status = next
  else delete query.status
  void router.replace({ query })
}

watch(
  () => route.query.q,
  (value) => {
    const next = normalizeTextFilter(value)
    if (search.value !== next) search.value = next
  }
)

watch(search, (value) => {
  const current = normalizeTextFilter(route.query.q)
  if (current === value) return
  const query = { ...route.query }
  if (value) query.q = value
  else delete query.q
  void router.replace({ query })
})
</script>

<template>
  <div class="page">
    <!-- Header -->
    <header class="page-header">
      <div class="header-left">
        <div class="page-icon">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71"/></svg>
        </div>
        <div>
          <h1>Apps</h1>
          <p class="subtitle">Create apps that everyone can interact with.</p>
        </div>
      </div>
      <div class="header-actions">
        <button type="button" class="btn btn-primary" @click="isCreateModalOpen = true">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          Create New App
        </button>
      </div>
    </header>

    <!-- Filters -->
    <div class="filters">
      <div class="search-wrap">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input v-model.trim="search" type="search" placeholder="Search apps" />
      </div>
      <div class="status-pills">
        <button type="button" :class="{ active: statusFilter === '' }" @click="setStatusFilter('')">All</button>
        <button type="button" :class="{ active: statusFilter === 'healthy' }" @click="setStatusFilter('healthy')">Healthy</button>
        <button type="button" :class="{ active: statusFilter === 'disabled' }" @click="setStatusFilter('disabled')">Disabled</button>
      </div>
      <LoadingSpinner v-if="appsQuery.isFetching.value" />
    </div>

    <ErrorBanner v-if="appsQuery.error.value" :message="appsQuery.error.value.message" />

    <!-- App cards -->
    <div v-if="filteredApps.length" class="app-list">
      <RouterLink
        v-for="app in filteredApps"
        :key="app.app_id"
        class="app-card card"
        :to="`/apps/${app.slug}`"
      >
        <div class="app-card-body">
          <div class="app-card-top">
            <h3 class="app-name">{{ app.slug }}</h3>
            <div class="app-badges">
              <span class="badge" :class="app.disabled ? 'badge-muted' : 'badge-green'">
                <span class="badge-dot" />
                {{ app.disabled ? 'Disabled' : 'Active' }}
              </span>
            </div>
          </div>
          <p v-if="app.description" class="app-desc">{{ app.description }}</p>
        </div>
        <div class="app-card-bar">
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: app.disabled ? '0%' : '100%', background: app.disabled ? 'var(--text-tertiary)' : 'var(--accent-green)' }"/>
          </div>
        </div>
      </RouterLink>
    </div>

    <div v-else-if="!appsQuery.isFetching.value" class="empty-state card">
      <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--text-tertiary)" stroke-width="1.5"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
      <h3>No apps found</h3>
      <p>Create a new app or clear the search filter.</p>
    </div>

    <CreateAppModal :open="isCreateModalOpen" @close="isCreateModalOpen = false" @created="handleCreated" />
  </div>
</template>

<style scoped>
.page {
  display: grid;
  gap: 1.25rem;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
}

.header-left {
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
  flex-shrink: 0;
  box-shadow: 0 0 16px color-mix(in srgb, var(--accent-blue) 8%, transparent);
}

.subtitle {
  margin: 0.1rem 0 0;
  color: var(--text-secondary);
  font-size: 0.82rem;
}

.header-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 1rem;
  border-radius: var(--radius-sm);
  font-size: 0.85rem;
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

.btn-primary:hover {
  background: color-mix(in srgb, var(--accent-blue) 85%, white);
  box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-blue) 30%, transparent);
  transform: translateY(-1px);
}

.btn-primary:active {
  transform: translateY(0);
}

/* Filters */
.filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.6rem;
}

.search-wrap {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
  color: var(--text-secondary);
  min-width: 240px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.search-wrap:focus-within {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 10%, transparent);
}

.search-wrap input {
  border: none;
  background: transparent;
  outline: none;
  flex: 1;
  min-width: 0;
}

.status-pills {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
}

.status-pills button {
  border: none;
  background: transparent;
  color: var(--text-secondary);
  padding: 0.35rem 0.55rem;
  border-radius: 4px;
  font-size: 0.78rem;
  font-weight: 500;
  cursor: pointer;
}

.status-pills button.active {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  box-shadow: var(--shadow-soft);
}

/* App list */
.app-list {
  display: grid;
  gap: 0.65rem;
}

.app-card {
  padding: 1rem 1.15rem;
  display: grid;
  gap: 0.75rem;
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  transition: border-color var(--transition-base), box-shadow var(--transition-base), transform var(--transition-base);
  border-left: 2px solid transparent;
}

.app-card:hover {
  border-color: var(--border-strong);
  border-left-color: var(--accent-blue);
  box-shadow: var(--shadow-soft), var(--shadow-glow-blue);
  transform: translateX(2px);
}

.app-card:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--accent-blue) 60%, white);
  outline-offset: 2px;
}

.app-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.app-name {
  font-size: 1rem;
  font-weight: 600;
}

.app-badges {
  display: flex;
  gap: 0.4rem;
}

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

.badge-green {
  color: var(--accent-green);
  border-color: color-mix(in srgb, var(--accent-green) 25%, var(--border-default));
  background: color-mix(in srgb, var(--accent-green) 8%, transparent);
}

.badge-muted {
  color: var(--text-tertiary);
}

.badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.app-desc {
  margin: 0;
  color: var(--text-secondary);
  font-size: 0.82rem;
}

.app-card-bar {
  padding-top: 0.25rem;
}

.bar-track {
  height: 4px;
  border-radius: 2px;
  background: var(--bg-elevated);
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 500ms ease;
}

/* Empty state */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2.5rem 1rem;
  text-align: center;
  color: var(--text-tertiary);
}

.empty-state h3 { color: var(--text-secondary); }
.empty-state p { margin: 0; }

@media (max-width: 700px) {
  .page-header {
    flex-direction: column;
  }
  .search-wrap {
    min-width: 0;
    width: 100%;
  }
}
</style>
