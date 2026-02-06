<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const loginSlug = ref('')
const loginPassword = ref('')

const bootstrapToken = ref('')
const bootstrapSlug = ref('')
const bootstrapName = ref('')
const bootstrapPassword = ref('')

const busy = ref(false)
const errorMessage = ref('')

const redirectPath = computed(() => {
  const raw = route.query.redirect
  return typeof raw === 'string' && raw.startsWith('/') ? raw : '/home'
})

async function submitLogin(): Promise<void> {
  busy.value = true
  errorMessage.value = ''
  try {
    await auth.login(loginSlug.value, loginPassword.value)
    await router.replace(redirectPath.value)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Login failed'
  } finally {
    busy.value = false
  }
}

async function submitBootstrap(): Promise<void> {
  busy.value = true
  errorMessage.value = ''
  try {
    await auth.bootstrap(
      {
        slug: bootstrapSlug.value,
        name: bootstrapName.value,
        password: bootstrapPassword.value || undefined
      },
      bootstrapToken.value
    )
    await router.replace('/home')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Bootstrap failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-container">
      <!-- Logo area -->
      <div class="brand">
        <div class="logo">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polygon points="12 2 2 7 12 12 22 7 12 2" />
            <polyline points="2 17 12 22 22 17" />
            <polyline points="2 12 12 17 22 12" />
          </svg>
        </div>
        <h1>Tower</h1>
        <p>Run orchestration platform</p>
      </div>

      <!-- Error -->
      <div v-if="errorMessage" class="error-bar">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></svg>
        {{ errorMessage }}
      </div>

      <!-- Sign In -->
      <section class="auth-card">
        <h2>Sign In</h2>
        <form class="form" @submit.prevent="submitLogin">
          <label class="field">
            <span>Team slug</span>
            <input v-model.trim="loginSlug" placeholder="my-team" required />
          </label>
          <label class="field">
            <span>Password</span>
            <input v-model="loginPassword" type="password" placeholder="Enter password" required />
          </label>
          <button :disabled="busy" type="submit" class="submit-btn">
            {{ busy ? 'Signing in...' : 'Sign In' }}
          </button>
        </form>
      </section>

      <!-- Bootstrap -->
      <section class="auth-card bootstrap">
        <h2>Bootstrap Team</h2>
        <p class="hint">First-time setup with a bootstrap token.</p>
        <form class="form" @submit.prevent="submitBootstrap">
          <label class="field">
            <span>Bootstrap token</span>
            <input v-model="bootstrapToken" placeholder="tok_..." required />
          </label>
          <label class="field">
            <span>Team slug</span>
            <input v-model.trim="bootstrapSlug" placeholder="my-team" required />
          </label>
          <label class="field">
            <span>Team name</span>
            <input v-model.trim="bootstrapName" placeholder="My Team" required />
          </label>
          <label class="field">
            <span>Password <span class="optional">(optional)</span></span>
            <input v-model="bootstrapPassword" type="password" placeholder="Choose a password" />
          </label>
          <button :disabled="busy" type="submit" class="submit-btn secondary">
            {{ busy ? 'Setting up...' : 'Bootstrap Team' }}
          </button>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 2rem 1rem;
  background: var(--bg-primary);
}

.login-container {
  width: min(420px, 100%);
  display: grid;
  gap: 1.25rem;
}

.brand {
  text-align: center;
  padding-bottom: 0.5rem;
}

.logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 14px;
  background: linear-gradient(135deg, var(--accent-blue), var(--accent-purple));
  color: white;
  margin-bottom: 0.75rem;
}

.brand h1 {
  margin: 0;
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--text-primary);
}

.brand p {
  margin: 0.25rem 0 0;
  color: var(--text-tertiary);
  font-size: 0.85rem;
}

.error-bar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 0.75rem;
  border-radius: var(--radius-sm);
  border: 1px solid color-mix(in srgb, var(--accent-red) 40%, var(--border-default));
  background: color-mix(in srgb, var(--accent-red) 8%, transparent);
  color: var(--accent-red);
  font-size: 0.85rem;
}

.auth-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 1.25rem;
  display: grid;
  gap: 0.85rem;
}

.auth-card h2 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
}

.hint {
  margin: -0.4rem 0 0;
  color: var(--text-tertiary);
  font-size: 0.8rem;
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

input {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  padding: 0.55rem 0.7rem;
  font-size: 0.88rem;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

input:focus {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
  outline: none;
}

input::placeholder {
  color: var(--text-tertiary);
}

.submit-btn {
  width: 100%;
  padding: 0.6rem;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent-blue);
  color: white;
  font-size: 0.88rem;
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

.submit-btn.secondary {
  background: color-mix(in srgb, var(--accent-blue) 18%, var(--bg-tertiary));
  color: var(--accent-blue);
  border: 1px solid color-mix(in srgb, var(--accent-blue) 30%, var(--border-default));
}

.submit-btn.secondary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-blue) 25%, var(--bg-tertiary));
}
</style>
