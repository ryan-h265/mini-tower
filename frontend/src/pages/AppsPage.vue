<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { apiClient } from '../api/client'
import type { AppResponse } from '../api/types'
import CreateAppModal from '../components/apps/CreateAppModal.vue'
import ErrorBanner from '../components/shared/ErrorBanner.vue'
import LoadingSpinner from '../components/shared/LoadingSpinner.vue'

const router = useRouter()
const search = ref('')
const isCreateModalOpen = ref(false)

const appsQuery = useQuery({
  queryKey: ['apps'],
  queryFn: () => apiClient.listApps()
})

const apps = computed(() => appsQuery.data.value?.apps ?? [])
const filteredApps = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return apps.value
  return apps.value.filter((app) => {
    const desc = app.description?.toLowerCase() ?? ''
    return app.slug.toLowerCase().includes(term) || desc.includes(term)
  })
})

function handleCreated(app: AppResponse): void {
  isCreateModalOpen.value = false
  void router.push(`/apps/${app.slug}`)
}
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
        <button type="button" class="btn btn-outline" @click="isCreateModalOpen = true">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10"/></svg>
          Deploy Example App
        </button>
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
      <LoadingSpinner v-if="appsQuery.isFetching.value" />
    </div>

    <ErrorBanner v-if="appsQuery.error.value" :message="appsQuery.error.value.message" />

    <!-- App cards -->
    <div v-if="filteredApps.length" class="app-list">
      <article v-for="app in filteredApps" :key="app.app_id" class="app-card card" @click="router.push(`/apps/${app.slug}`)">
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
      </article>
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
  background: var(--bg-tertiary);
  display: grid;
  place-items: center;
  color: var(--accent-blue);
  flex-shrink: 0;
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

.btn-outline {
  color: var(--text-secondary);
}

.btn-outline:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
}

.btn-primary {
  background: var(--accent-blue);
  border-color: var(--accent-blue);
  color: white;
}

.btn-primary:hover {
  background: color-mix(in srgb, var(--accent-blue) 85%, white);
}

/* Filters */
.filters {
  display: flex;
  align-items: center;
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
}

.search-wrap input {
  border: none;
  background: transparent;
  outline: none;
  flex: 1;
  min-width: 0;
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
  transition: border-color var(--transition-base), box-shadow var(--transition-base);
}

.app-card:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow-soft), var(--shadow-glow-blue);
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
