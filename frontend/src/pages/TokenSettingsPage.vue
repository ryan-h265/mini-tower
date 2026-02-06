<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { apiClient } from '../api/client'
import type { TokenRole } from '../api/types'
import ErrorBanner from '../components/shared/ErrorBanner.vue'

const auth = useAuthStore()

const name = ref('')
const role = ref<TokenRole>('member')
const busy = ref(false)
const errorMessage = ref('')
const createdToken = ref<string | null>(null)
const createdRole = ref<TokenRole | null>(null)
const copied = ref(false)

async function submit(): Promise<void> {
  busy.value = true
  errorMessage.value = ''
  createdToken.value = null
  createdRole.value = null
  copied.value = false

  try {
    const response = await apiClient.createToken({
      name: name.value.trim() || undefined,
      role: auth.isAdmin ? role.value : undefined
    })
    createdToken.value = response.token
    createdRole.value = response.role
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to create token'
  } finally {
    busy.value = false
  }
}

async function copyToken(): Promise<void> {
  if (!createdToken.value) {
    return
  }
  try {
    await navigator.clipboard.writeText(createdToken.value)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1200)
  } catch {
    errorMessage.value = 'Clipboard copy is unavailable in this browser context.'
  }
}
</script>

<template>
  <div class="page">
    <header class="page-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
        <path d="M7 11V7a5 5 0 0 1 10 0v4" />
      </svg>
      <div>
        <h1>Token Settings</h1>
        <p class="subtitle">Create scoped team tokens for local development and API automation.</p>
      </div>
    </header>

    <div class="info-bar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" /></svg>
      No list/revoke endpoint is available yet. This page only supports creating a new token and showing it once.
    </div>

    <ErrorBanner v-if="errorMessage" :message="errorMessage" />

    <section class="form-card">
      <h2>Create Token</h2>
      <form class="form" @submit.prevent="submit">
        <label class="field">
          <span>Name <span class="optional">(optional)</span></span>
          <input v-model="name" placeholder="deploy-bot" />
        </label>

        <label class="field">
          <span>Role</span>
          <select v-if="auth.isAdmin" v-model="role">
            <option value="member">member</option>
            <option value="admin">admin</option>
          </select>
          <input v-else value="member (fixed for non-admin)" disabled />
        </label>

        <button :disabled="busy" type="submit" class="submit-btn">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
          {{ busy ? 'Creating...' : 'Create Token' }}
        </button>
      </form>
    </section>

    <section v-if="createdToken" class="token-output">
      <header class="token-header">
        <div class="token-title">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="20 6 9 17 4 12" /></svg>
          <strong>Token Created</strong>
        </div>
        <span v-if="createdRole" class="role-chip">{{ createdRole }}</span>
      </header>

      <code class="token-value">{{ createdToken }}</code>

      <div class="token-actions">
        <button type="button" class="copy-btn" @click="copyToken">
          <svg v-if="!copied" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="20 6 9 17 4 12" /></svg>
          {{ copied ? 'Copied!' : 'Copy token' }}
        </button>
      </div>

      <div class="warning">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /><line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>
        Store this token now. It will not be shown again.
      </div>
    </section>
  </div>
</template>

<style scoped>
.page {
  display: grid;
  gap: 1rem;
  max-width: 560px;
}

.page-header {
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
  color: var(--text-primary);
}

.page-header svg {
  margin-top: 0.15rem;
  flex-shrink: 0;
}

.page-header h1 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
}

.subtitle {
  margin: 0.15rem 0 0;
  color: var(--text-tertiary);
  font-size: 0.82rem;
}

.info-bar {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.6rem 0.75rem;
  border-radius: var(--radius-sm);
  border: 1px solid color-mix(in srgb, var(--accent-blue) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-blue) 6%, transparent);
  color: color-mix(in srgb, var(--accent-blue) 75%, var(--text-primary));
  font-size: 0.82rem;
  line-height: 1.45;
}

.info-bar svg {
  flex-shrink: 0;
  margin-top: 0.1rem;
}

.form-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 1.25rem;
  display: grid;
  gap: 0.85rem;
}

.form-card h2 {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
}

.form {
  display: grid;
  gap: 0.65rem;
}

.field {
  display: grid;
  gap: 0.3rem;
}

.field > span {
  color: var(--text-secondary);
  font-size: 0.8rem;
  font-weight: 500;
}

.optional {
  color: var(--text-tertiary);
  font-weight: 400;
}

input, select {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  padding: 0.5rem 0.65rem;
  font-size: 0.85rem;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

input:focus, select:focus {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
  outline: none;
}

input::placeholder {
  color: var(--text-tertiary);
}

input:disabled {
  opacity: 0.5;
  cursor: default;
}

select {
  cursor: pointer;
}

.submit-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  width: fit-content;
  padding: 0.5rem 1rem;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent-blue);
  color: white;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: background var(--transition-fast);
  margin-top: 0.15rem;
}

.submit-btn:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-blue) 85%, white);
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.token-output {
  border: 1px solid color-mix(in srgb, var(--accent-green) 30%, var(--border-default));
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--accent-green) 4%, var(--bg-secondary));
  padding: 1rem;
  display: grid;
  gap: 0.65rem;
}

.token-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.token-title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: var(--accent-green);
}

.role-chip {
  display: inline-flex;
  padding: 0.15rem 0.55rem;
  border-radius: 999px;
  background: color-mix(in srgb, var(--accent-purple) 12%, transparent);
  color: var(--accent-purple);
  font-size: 0.75rem;
  font-weight: 600;
}

.token-value {
  display: block;
  font-family: var(--font-mono);
  font-size: 0.8rem;
  word-break: break-all;
  padding: 0.6rem 0.7rem;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  color: var(--text-primary);
}

.token-actions {
  display: flex;
  gap: 0.5rem;
}

.copy-btn {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.7rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.copy-btn:hover {
  border-color: var(--accent-blue);
  color: var(--accent-blue);
}

.warning {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: var(--accent-yellow);
  font-size: 0.8rem;
  font-weight: 500;
}

.warning svg {
  flex-shrink: 0;
}
</style>
