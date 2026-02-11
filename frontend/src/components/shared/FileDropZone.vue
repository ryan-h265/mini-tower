<script setup lang="ts">
import { ref } from 'vue'

const emit = defineEmits<{ 'file-selected': [File] }>()
const dragging = ref(false)

function onFileList(files: FileList | null): void {
  const file = files?.item(0)
  if (!file) {
    return
  }
  emit('file-selected', file)
}
</script>

<template>
  <label
    class="zone"
    :class="{ dragging }"
    @dragenter.prevent="dragging = true"
    @dragover.prevent
    @dragleave.prevent="dragging = false"
    @drop.prevent="dragging = false; onFileList($event.dataTransfer?.files ?? null)"
  >
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="icon">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="17 8 12 3 7 8" />
      <line x1="12" y1="3" x2="12" y2="15" />
    </svg>
    <span class="label">Drop artifact here or click to choose</span>
    <span class="hint">.tar.gz, .zip, or binary</span>
    <input type="file" @change="onFileList(($event.target as HTMLInputElement).files)" />
  </label>
</template>

<style scoped>
.zone {
  border: 2px dashed var(--border-default);
  border-radius: var(--radius-sm);
  min-height: 140px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  text-align: center;
  cursor: pointer;
  transition: all var(--transition-fast);
  background: color-mix(in srgb, var(--bg-tertiary) 50%, transparent);
}

.zone:hover {
  border-color: var(--accent-blue);
  background: color-mix(in srgb, var(--accent-blue) 4%, transparent);
}

.zone.dragging {
  border-color: var(--accent-blue);
  background: color-mix(in srgb, var(--accent-blue) 8%, transparent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
}

.icon {
  color: var(--text-tertiary);
  margin-bottom: 0.15rem;
}

.dragging .icon {
  color: var(--accent-blue);
}

.label {
  color: var(--text-secondary);
  font-size: 0.85rem;
  font-weight: 500;
}

.hint {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

input {
  display: none;
}
</style>
