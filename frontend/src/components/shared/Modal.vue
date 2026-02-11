<script setup lang="ts">
defineProps<{ open: boolean }>()
defineEmits<{ close: [] }>()
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="backdrop" @click.self="$emit('close')">
        <div class="modal card">
          <button type="button" class="close-btn" @click="$emit('close')">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
          <slot />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  backdrop-filter: blur(8px) saturate(1.2);
  -webkit-backdrop-filter: blur(8px) saturate(1.2);
  display: grid;
  place-items: center;
  padding: 1rem;
  z-index: 100;
}

.modal {
  position: relative;
  width: min(560px, 100%);
  padding: 1.25rem;
  max-height: 90vh;
  overflow-y: auto;
  border-color: var(--glass-border);
  box-shadow:
    0 24px 80px rgba(0, 0, 0, 0.4),
    0 0 0 1px var(--glass-border),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.close-btn {
  position: absolute;
  top: 0.85rem;
  right: 0.85rem;
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.close-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  transform: rotate(90deg);
}

.close-btn svg {
  transition: transform var(--transition-base);
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 250ms ease;
}

.modal-enter-active .modal,
.modal-leave-active .modal {
  transition: transform 300ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 250ms ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal {
  transform: scale(0.92) translateY(12px);
  opacity: 0;
}

.modal-leave-to .modal {
  transform: scale(0.96) translateY(-4px);
  opacity: 0;
}
</style>
