<script setup lang="ts">
defineProps<{ title: string; message?: string }>()
</script>

<template>
  <div class="empty">
    <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="icon">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
    <h3>{{ title }}</h3>
    <p v-if="message">{{ message }}</p>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2.5rem 1.5rem;
  text-align: center;
}

.icon {
  color: var(--text-tertiary);
  opacity: 0.5;
  margin-bottom: 0.25rem;
}

h3 {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--text-secondary);
}

p {
  margin: 0;
  font-size: 0.82rem;
  color: var(--text-tertiary);
  max-width: 320px;
  line-height: 1.5;
}
</style>
