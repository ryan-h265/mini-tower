<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from '../shared/Modal.vue'
import ErrorBanner from '../shared/ErrorBanner.vue'
import type { CreateRunRequest, VersionResponse } from '../../api/types'

interface RunSeed {
  version_no?: number
  input?: Record<string, unknown>
  priority?: number
  max_retries?: number
}

interface SchemaField {
  name: string
  kind: 'string' | 'number' | 'integer' | 'boolean'
  required: boolean
  description?: string
  enumValues?: Array<string | number | boolean>
  defaultValue?: unknown
}

const props = withDefaults(
  defineProps<{
    open: boolean
    versions: VersionResponse[]
    seed?: RunSeed
    busy?: boolean
    errorMessage?: string
  }>(),
  { busy: false, errorMessage: '' }
)

const emit = defineEmits<{ close: []; submit: [CreateRunRequest] }>()

const selectedVersionNo = ref<number | null>(null)
const priority = ref(0)
const maxRetries = ref(0)
const formValues = ref<Record<string, string | number | boolean | ''>>({})
const localError = ref('')

const selectedVersion = computed(() => props.versions.find(v => v.version_no === selectedVersionNo.value) ?? null)
const selectedSchema = computed(() => {
  const schema = selectedVersion.value?.params_schema
  if (!schema || typeof schema !== 'object' || Array.isArray(schema)) return null
  return schema as Record<string, unknown>
})

function normalizeType(rawType: unknown): SchemaField['kind'] {
  if (typeof rawType === 'string') {
    if (rawType === 'number' || rawType === 'integer' || rawType === 'boolean') return rawType
    return 'string'
  }
  if (Array.isArray(rawType)) {
    for (const entry of rawType) {
      if (entry === 'number' || entry === 'integer' || entry === 'boolean' || entry === 'string') return entry
    }
  }
  return 'string'
}

const schemaFields = computed<SchemaField[]>(() => {
  const schema = selectedSchema.value
  if (!schema || schema.type !== 'object') return []
  const properties = schema.properties
  if (!properties || typeof properties !== 'object' || Array.isArray(properties)) return []
  const requiredRaw = Array.isArray(schema.required) ? schema.required : []
  const requiredNames = new Set(requiredRaw.filter((e): e is string => typeof e === 'string'))
  return Object.entries(properties)
    .filter((entry): entry is [string, Record<string, unknown>] => {
      const v = entry[1]; return typeof v === 'object' && v !== null && !Array.isArray(v)
    })
    .map(([name, details]) => {
      const enumValues = Array.isArray(details.enum) ? details.enum.filter((v): v is string | number | boolean => typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') : undefined
      return { name, kind: normalizeType(details.type), required: requiredNames.has(name), description: typeof details.description === 'string' ? details.description : undefined, enumValues: enumValues && enumValues.length > 0 ? enumValues : undefined, defaultValue: details.default }
    })
})

const hasSchemaFields = computed(() => schemaFields.value.length > 0)
const displayError = computed(() => localError.value || props.errorMessage)

function defaultFieldValue(kind: SchemaField['kind'], schemaDefault?: unknown): string | number | boolean | '' {
  if (schemaDefault !== undefined && schemaDefault !== null) {
    if (kind === 'boolean') return Boolean(schemaDefault)
    if (kind === 'number' || kind === 'integer') return typeof schemaDefault === 'number' ? schemaDefault : Number(schemaDefault)
    return String(schemaDefault)
  }
  if (kind === 'boolean') return false
  return ''
}

function syncFormValues(baseInput: Record<string, unknown>): void {
  const next: Record<string, string | number | boolean | ''> = {}
  for (const field of schemaFields.value) {
    const value = baseInput[field.name]
    if (value === undefined || value === null) { next[field.name] = defaultFieldValue(field.kind, field.defaultValue); continue }
    if (field.kind === 'boolean') { next[field.name] = Boolean(value); continue }
    if (field.kind === 'number' || field.kind === 'integer') { next[field.name] = typeof value === 'number' ? value : Number(value); continue }
    next[field.name] = String(value)
  }
  formValues.value = next
}

function initializeFromProps(): void {
  const latest = props.versions.length > 0 ? props.versions[0].version_no : null
  selectedVersionNo.value = props.seed?.version_no ?? latest
  priority.value = props.seed?.priority ?? 0
  maxRetries.value = props.seed?.max_retries ?? 0
  localError.value = ''
}

function formInputSnapshot(): Record<string, unknown> {
  const snapshot: Record<string, unknown> = {}
  for (const [name, value] of Object.entries(formValues.value)) {
    if (value === '' || value === undefined) continue
    snapshot[name] = value
  }
  return snapshot
}

function buildInputFromForm(): Record<string, unknown> {
  const output: Record<string, unknown> = {}
  for (const field of schemaFields.value) {
    const raw = formValues.value[field.name]
    if (raw === '' || raw === undefined) { if (field.required) throw new Error(`Field "${field.name}" is required.`); continue }
    if (field.kind === 'boolean') { output[field.name] = Boolean(raw); continue }
    if (field.kind === 'integer') { const v = typeof raw === 'number' ? raw : Number(raw); if (!Number.isInteger(v)) throw new Error(`Field "${field.name}" must be an integer.`); output[field.name] = v; continue }
    if (field.kind === 'number') { const v = typeof raw === 'number' ? raw : Number(raw); if (Number.isNaN(v)) throw new Error(`Field "${field.name}" must be a number.`); output[field.name] = v; continue }
    output[field.name] = String(raw)
  }
  return output
}

watch(() => props.open, (open) => {
  if (!open) return
  initializeFromProps(); syncFormValues(props.seed?.input ?? {})
})

watch(schemaFields, () => {
  syncFormValues(formInputSnapshot())
})

watch(selectedVersionNo, () => {
  syncFormValues(formInputSnapshot())
})

function submit(): void {
  localError.value = ''
  if (!selectedVersionNo.value) { localError.value = 'Select a version before creating a run.'; return }
  try {
    const input = buildInputFromForm()
    emit('submit', { version_no: selectedVersionNo.value, input, priority: priority.value, max_retries: maxRetries.value })
  } catch (error) { localError.value = error instanceof Error ? error.message : 'Unable to submit run' }
}
</script>

<template>
  <Modal :open="open" @close="$emit('close')">
    <div class="stack">
      <h2>Create a Run</h2>

      <ErrorBanner v-if="displayError" :message="displayError" />

      <!-- App/Version selector -->
      <label class="field">
        <span>App Name</span>
        <select v-model.number="selectedVersionNo" :disabled="busy || versions.length === 0" class="select">
          <option :value="null" disabled>Select a version</option>
          <option v-for="version in versions" :key="version.version_id" :value="version.version_no">
            v{{ version.version_no }} - {{ version.entrypoint }}{{ version.towerfile_toml ? ' (Towerfile)' : '' }}
          </option>
        </select>
      </label>

      <!-- Default params display -->
      <div v-if="hasSchemaFields" class="params-section">
        <div class="params-header">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
          <span class="params-title">DEFAULT PARAMETERS</span>
        </div>
        <div class="param-list">
          <div v-for="field in schemaFields" :key="field.name" class="param-row">
            <div class="param-info">
              <div class="param-name">
                <strong>{{ field.name }}</strong>
                <span v-if="formValues[field.name] !== '' && formValues[field.name] !== undefined"> = {{ formValues[field.name] }}</span>
              </div>
              <span v-if="field.description" class="param-desc">{{ field.description }}</span>
            </div>
            <div class="param-action">
              <template v-if="field.enumValues?.length">
                <select v-model="formValues[field.name]" :disabled="busy" class="param-input">
                  <option value="">Select...</option>
                  <option v-for="opt in field.enumValues" :key="String(opt)" :value="opt">{{ opt }}</option>
                </select>
              </template>
              <template v-else-if="field.kind === 'boolean'">
                <label class="checkbox-row"><input v-model="formValues[field.name]" type="checkbox" :disabled="busy" /><span>Enabled</span></label>
              </template>
              <template v-else-if="field.kind === 'integer' || field.kind === 'number'">
                <input v-model.number="formValues[field.name]" type="number" :step="field.kind === 'integer' ? 1 : 'any'" :disabled="busy" class="param-input" />
              </template>
              <template v-else>
                <input v-model="formValues[field.name]" type="text" :disabled="busy" class="param-input" />
              </template>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="params-empty">
        This version has no configurable parameters.
      </div>

      <div class="divider"/>

      <!-- Submit -->
      <button type="button" class="submit-btn" :disabled="busy || versions.length === 0" @click="submit">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        {{ busy ? 'Submitting...' : 'Run App' }}
      </button>
    </div>
  </Modal>
</template>

<style scoped>
.stack { display: grid; gap: 0.85rem; }

h2 { margin: 0; font-size: 1.15rem; }

.field { display: grid; gap: 0.3rem; }
.field > span { color: var(--text-secondary); font-size: 0.82rem; font-weight: 500; }

input, select {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  padding: 0.55rem 0.7rem;
}

.select { appearance: none; cursor: pointer; }

input:focus, select:focus {
  border-color: var(--accent-blue);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-blue) 15%, transparent);
  outline: none;
}

/* Params section */
.params-section {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  overflow: hidden;
}

.params-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.55rem 0.75rem;
  border-bottom: 1px solid var(--border-default);
  color: var(--text-secondary);
}

.params-title {
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.param-list { display: grid; gap: 0; }

.param-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.55rem 0.75rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
}

.param-row:last-child { border-bottom: none; }

.param-info { display: grid; gap: 0.15rem; min-width: 0; }
.param-name { font-size: 0.85rem; }
.param-desc { color: var(--text-tertiary); font-size: 0.75rem; }

.param-action { flex-shrink: 0; }

.param-input {
  width: 120px;
  padding: 0.3rem 0.5rem;
  font-size: 0.82rem;
  background: var(--bg-elevated);
}

.checkbox-row { display: inline-flex; align-items: center; gap: 0.35rem; font-size: 0.82rem; color: var(--text-secondary); }

.params-empty {
  border: 1px dashed var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 0.82rem;
  padding: 0.7rem 0.8rem;
}

.divider {
  height: 1px;
  background: var(--border-default);
}

.submit-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
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

.submit-btn:hover:not(:disabled) { background: color-mix(in srgb, var(--accent-blue) 85%, white); }
.submit-btn:disabled { opacity: 0.5; cursor: default; }
</style>
