<script setup lang="ts">
import type { RunStatus } from '../../api/types'

const props = defineProps<{ status: RunStatus }>()

function statusClass(status: RunStatus): string {
  switch (status) {
    case 'completed':
      return 'ok'
    case 'failed':
    case 'dead':
      return 'error'
    case 'cancelled':
    case 'cancelling':
      return 'warn'
    case 'running':
    case 'leased':
      return 'info'
    case 'queued':
    default:
      return 'neutral'
  }
}
</script>

<template>
  <span class="badge" :class="statusClass(props.status)">
    <span class="dot" />
    {{ props.status }}
  </span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border-radius: 999px;
  padding: 0.18rem 0.6rem 0.18rem 0.45rem;
  font-size: 0.75rem;
  font-weight: 500;
  text-transform: capitalize;
  letter-spacing: 0.01em;
  background: color-mix(in srgb, var(--border-default) 40%, transparent);
  color: var(--text-secondary);
}

.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
  background: currentColor;
}

.ok {
  color: var(--accent-green);
  background: color-mix(in srgb, var(--accent-green) 12%, transparent);
}

.error {
  color: var(--accent-red);
  background: color-mix(in srgb, var(--accent-red) 12%, transparent);
}

.warn {
  color: var(--accent-yellow);
  background: color-mix(in srgb, var(--accent-yellow) 12%, transparent);
}

.info {
  color: var(--accent-blue);
  background: color-mix(in srgb, var(--accent-blue) 12%, transparent);
}

.neutral {
  color: var(--text-tertiary);
}
</style>
