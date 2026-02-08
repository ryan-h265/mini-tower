<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import Modal from '../shared/Modal.vue'
import ErrorBanner from '../shared/ErrorBanner.vue'
import { apiClient } from '../../api/client'
import type { AppResponse, ListAppsResponse } from '../../api/types'
import { useToast } from '../../composables/useToast'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; created: [AppResponse] }>()

const queryClient = useQueryClient()
const toast = useToast()
const slug = ref('')
const description = ref('')
const errorMessage = ref('')

const slugRegex = /^[a-z][a-z0-9-]{2,31}$/
const reservedSlugs = new Set(['api', 'admin', 'system', 'internal', 'default'])

const canSubmit = computed(() => slug.value.trim().length > 0 && !createMutation.isPending.value)

function resetForm(): void {
  slug.value = ''
  description.value = ''
  errorMessage.value = ''
}

watch(() => props.open, (open) => { if (!open) resetForm() })

function validateSlug(input: string): string | null {
  if (!slugRegex.test(input)) return 'Slug must be 3-32 chars, start with a letter, and use lowercase letters, numbers, or hyphens.'
  if (reservedSlugs.has(input)) return 'This slug is reserved.'
  return null
}

const createMutation = useMutation({
  mutationFn: () => apiClient.createApp({ slug: slug.value.trim(), description: description.value.trim() || undefined }),
  onMutate: async () => {
    await queryClient.cancelQueries({ queryKey: ['apps'] })
    const previous = queryClient.getQueryData<ListAppsResponse>(['apps'])
    const optimistic: AppResponse = {
      app_id: -Date.now(), slug: slug.value.trim(), description: description.value.trim() || undefined,
      disabled: false, created_at: new Date().toISOString(), updated_at: new Date().toISOString()
    }
    queryClient.setQueryData<ListAppsResponse>(['apps'], (current) => {
      const apps = current?.apps ?? []
      return { apps: [...apps, optimistic].sort((a, b) => a.slug.localeCompare(b.slug)) }
    })
    return { previous }
  },
  onError: (error, _variables, context) => {
    if (context?.previous) queryClient.setQueryData(['apps'], context.previous)
    errorMessage.value = error instanceof Error ? error.message : 'Failed to create app'
    toast.error(errorMessage.value)
  },
  onSuccess: (app) => {
    toast.success(`App "${app.slug}" created.`)
    emit('created', app)
    emit('close')
    resetForm()
  },
  onSettled: async () => { await queryClient.invalidateQueries({ queryKey: ['apps'] }) }
})

function submit(): void {
  errorMessage.value = ''
  const validationError = validateSlug(slug.value.trim())
  if (validationError) { errorMessage.value = validationError; return }
  void createMutation.mutateAsync()
}
</script>

<template>
  <Modal :open="open" @close="$emit('close')">
    <div class="stack">
      <h2>Create a New App</h2>

      <ErrorBanner v-if="errorMessage" :message="errorMessage" />

      <label class="field">
        <span>Name <span class="required">*</span></span>
        <input v-model.trim="slug" autocomplete="off" placeholder="My New App" />
      </label>

      <label class="field">
        <span>Short description</span>
        <input v-model="description" placeholder="Short description of what your app does" />
      </label>

      <button type="button" class="submit-btn" :disabled="!canSubmit" @click="submit">
        {{ createMutation.isPending.value ? 'Creating...' : 'Create' }}
      </button>
    </div>
  </Modal>
</template>

<style scoped>
.stack {
  display: grid;
  gap: 1rem;
}

h2 { margin: 0; font-size: 1.15rem; }

.field {
  display: grid;
  gap: 0.35rem;
}

.field > span {
  color: var(--text-secondary);
  font-size: 0.82rem;
  font-weight: 500;
}

.required { color: var(--accent-red); }

input {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  padding: 0.6rem 0.75rem;
  font-size: 0.9rem;
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
  padding: 0.65rem;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent-blue);
  color: white;
  font-size: 0.9rem;
  font-weight: 600;
  cursor: pointer;
  transition: background var(--transition-fast);
}

.submit-btn:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-blue) 85%, white);
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>
