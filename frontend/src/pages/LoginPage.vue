<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiClient } from '../api/client'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const loginSlug = ref('')
const loginPassword = ref('')

const signupSlug = ref('')
const signupName = ref('')
const signupPassword = ref('')
const signupConfirmPassword = ref('')

const busy = ref(false)
const errorMessage = ref('')
const signupEnabled = ref(true)

const redirectPath = computed(() => {
  const raw = route.query.redirect
  return typeof raw === 'string' && raw.startsWith('/') ? raw : '/home'
})

onMounted(async () => {
  try {
    const options = await apiClient.getAuthOptions()
    signupEnabled.value = options.signup_enabled
  } catch {
    // Backward-compatible fallback for older API servers.
    signupEnabled.value = true
  }
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

async function submitSignup(): Promise<void> {
  if (signupPassword.value !== signupConfirmPassword.value) {
    errorMessage.value = 'Passwords do not match'
    return
  }

  busy.value = true
  errorMessage.value = ''
  try {
    await auth.signup({
      slug: signupSlug.value,
      name: signupName.value,
      password: signupPassword.value
    })
    await router.replace(redirectPath.value)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Signup failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-bg">
      <div class="grid-pattern" />
      <div class="orb orb-1" />
      <div class="orb orb-2" />
      <div class="orb orb-3" />
    </div>
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
        <h1>MiniTower</h1>
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

      <!-- Public Signup -->
      <section v-if="signupEnabled" class="auth-card signup">
        <h2>Create Team</h2>
        <p class="hint">Create a new team workspace with an admin account.</p>
        <form class="form" @submit.prevent="submitSignup">
          <label class="field">
            <span>Team slug</span>
            <input v-model.trim="signupSlug" placeholder="my-team" required />
          </label>
          <label class="field">
            <span>Team name</span>
            <input v-model.trim="signupName" placeholder="My Team" required />
          </label>
          <label class="field">
            <span>Password</span>
            <input v-model="signupPassword" type="password" placeholder="Choose a password" required />
          </label>
          <label class="field">
            <span>Confirm password</span>
            <input v-model="signupConfirmPassword" type="password" placeholder="Repeat password" required />
          </label>
          <button :disabled="busy" type="submit" class="submit-btn secondary">
            {{ busy ? 'Creating team...' : 'Create Team' }}
          </button>
        </form>
      </section>

      <section v-else class="auth-card signup">
        <h2>Team Signup Disabled</h2>
        <p class="hint">Ask your operator to provide credentials or enable public signup.</p>
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
  position: relative;
  overflow: hidden;
}

.login-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}

.grid-pattern {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(color-mix(in srgb, var(--border-default) 30%, transparent) 1px, transparent 1px),
    linear-gradient(90deg, color-mix(in srgb, var(--border-default) 30%, transparent) 1px, transparent 1px);
  background-size: 60px 60px;
  mask-image: radial-gradient(ellipse 60% 50% at 50% 50%, black 20%, transparent 70%);
  -webkit-mask-image: radial-gradient(ellipse 60% 50% at 50% 50%, black 20%, transparent 70%);
}

.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.4;
}

.orb-1 {
  width: 400px;
  height: 400px;
  background: var(--accent-blue);
  top: -10%;
  right: 10%;
  animation: float 12s ease-in-out infinite;
}

.orb-2 {
  width: 300px;
  height: 300px;
  background: var(--accent-purple);
  bottom: -5%;
  left: 5%;
  animation: float 15s ease-in-out infinite 2s;
}

.orb-3 {
  width: 250px;
  height: 250px;
  background: var(--accent-cyan);
  top: 40%;
  left: 50%;
  animation: float 18s ease-in-out infinite 4s;
}

.login-container {
  width: min(420px, 100%);
  display: grid;
  gap: 1.25rem;
  position: relative;
  z-index: 1;
  animation: fadeInUp 600ms ease both;
}

.brand {
  text-align: center;
  padding-bottom: 0.5rem;
}

.logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 60px;
  height: 60px;
  border-radius: 16px;
  background: linear-gradient(135deg, var(--accent-blue), var(--accent-purple));
  color: white;
  margin-bottom: 0.75rem;
  box-shadow:
    0 8px 32px color-mix(in srgb, var(--accent-blue) 30%, transparent),
    0 0 0 1px rgba(255, 255, 255, 0.1) inset;
  animation: float 6s ease-in-out infinite;
}

.brand h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.02em;
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
  background: color-mix(in srgb, var(--accent-red) 8%, var(--bg-secondary));
  color: var(--accent-red);
  font-size: 0.85rem;
  backdrop-filter: blur(8px);
}

.auth-card {
  background: var(--glass-bg);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-lg);
  padding: 1.35rem;
  display: grid;
  gap: 0.85rem;
  backdrop-filter: var(--glass-blur);
  -webkit-backdrop-filter: var(--glass-blur);
  box-shadow:
    var(--shadow-elevated),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
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

input {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--bg-tertiary) 80%, transparent);
  color: var(--text-primary);
  padding: 0.6rem 0.75rem;
  font-size: 0.88rem;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast), background var(--transition-fast);
}

input:focus {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
  background: var(--bg-tertiary);
  outline: none;
}

input::placeholder {
  color: var(--text-tertiary);
}

.submit-btn {
  width: 100%;
  padding: 0.65rem;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent-blue);
  color: white;
  font-size: 0.88rem;
  font-weight: 600;
  cursor: pointer;
  transition: background var(--transition-fast), box-shadow var(--transition-fast), transform var(--transition-fast);
  margin-top: 0.15rem;
}

.submit-btn:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-blue) 85%, white);
  box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-blue) 30%, transparent);
  transform: translateY(-1px);
}

.submit-btn:active:not(:disabled) {
  transform: translateY(0);
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
  box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-blue) 15%, transparent);
}
</style>
