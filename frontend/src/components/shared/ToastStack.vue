<script setup lang="ts">
import { useToast } from '../../composables/useToast'

const { toasts, remove } = useToast()
</script>

<template>
  <div class="toast-root" aria-live="polite" aria-atomic="true">
    <TransitionGroup name="toast" tag="div" class="toast-list">
      <div v-for="toast in toasts" :key="toast.id" class="toast" :class="`toast-${toast.kind}`" role="status">
        <span class="toast-icon" aria-hidden="true">{{ toast.kind === 'success' ? '✓' : toast.kind === 'error' ? '!' : 'i' }}</span>
        <span class="toast-message">{{ toast.message }}</span>
        <button type="button" class="toast-dismiss" aria-label="Dismiss notification" @click="remove(toast.id)">×</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-root {
  position: fixed;
  top: 0.85rem;
  right: 0.9rem;
  z-index: 60;
  pointer-events: none;
}

.toast-list {
  display: grid;
  gap: 0.45rem;
  width: min(360px, calc(100vw - 1.8rem));
}

.toast {
  pointer-events: auto;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 0.55rem;
  border: 1px solid var(--border-default);
  border-left-width: 3px;
  border-radius: var(--radius-sm);
  background: var(--gradient-surface);
  box-shadow: var(--shadow-card);
  padding: 0.5rem 0.6rem;
}

.toast-success {
  border-left-color: var(--accent-green);
}

.toast-error {
  border-left-color: var(--accent-red);
}

.toast-info {
  border-left-color: var(--accent-blue);
}

.toast-icon {
  width: 1.1rem;
  height: 1.1rem;
  border-radius: 999px;
  display: inline-grid;
  place-items: center;
  font-size: 0.72rem;
  font-weight: 700;
  color: var(--text-primary);
  background: color-mix(in srgb, var(--bg-tertiary) 78%, white);
}

.toast-message {
  color: var(--text-secondary);
  font-size: 0.8rem;
  line-height: 1.35;
}

.toast-dismiss {
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  width: 1.1rem;
  height: 1.1rem;
  display: inline-grid;
  place-items: center;
  border-radius: 4px;
  cursor: pointer;
}

.toast-dismiss:hover {
  color: var(--text-primary);
  background: color-mix(in srgb, var(--bg-tertiary) 70%, transparent);
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (max-width: 700px) {
  .toast-root {
    top: auto;
    bottom: 0.85rem;
    right: 0.65rem;
    left: 0.65rem;
  }

  .toast-list {
    width: 100%;
  }
}
</style>
