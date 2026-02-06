<template>
  <span class="spinner" aria-label="Loading" />
</template>

<style scoped>
.spinner {
  display: inline-block;
  width: 1rem;
  height: 1rem;
  border-radius: 50%;
  border: 2px solid color-mix(in srgb, var(--accent-blue) 20%, transparent);
  border-top-color: var(--accent-blue);
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
