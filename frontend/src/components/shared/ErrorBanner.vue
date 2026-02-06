<script setup lang="ts">
defineProps<{ message: string }>()
</script>

<template>
  <div class="error-banner">
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="10" />
      <line x1="12" y1="8" x2="12" y2="12" />
      <line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
    <span>{{ message }}</span>
  </div>
</template>

<style scoped>
.error-banner {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  margin: 0;
  padding: 0.6rem 0.75rem;
  border-radius: var(--radius-sm);
  border: 1px solid color-mix(in srgb, var(--accent-red) 40%, var(--border-default));
  background: color-mix(in srgb, var(--accent-red) 8%, transparent);
  color: var(--accent-red);
  font-size: 0.85rem;
  line-height: 1.45;
}

.error-banner svg {
  flex-shrink: 0;
  margin-top: 0.1rem;
}
</style>
