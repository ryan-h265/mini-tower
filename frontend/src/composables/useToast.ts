import { readonly, ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const toasts = ref<Toast[]>([])
let nextToastID = 1
const dismissTimers = new Map<number, number>()

function remove(id: number): void {
  const timer = dismissTimers.get(id)
  if (timer !== undefined) {
    window.clearTimeout(timer)
    dismissTimers.delete(id)
  }
  toasts.value = toasts.value.filter(toast => toast.id !== id)
}

function push(kind: ToastKind, message: string, durationMs?: number): number {
  const id = nextToastID++
  toasts.value = [...toasts.value, { id, kind, message }]
  const ttl = durationMs ?? (kind === 'error' ? 5000 : 3500)
  const timer = window.setTimeout(() => {
    remove(id)
  }, ttl)
  dismissTimers.set(id, timer)
  return id
}

function success(message: string, durationMs?: number): number {
  return push('success', message, durationMs)
}

function error(message: string, durationMs?: number): number {
  return push('error', message, durationMs)
}

function info(message: string, durationMs?: number): number {
  return push('info', message, durationMs)
}

export function useToast() {
  return {
    toasts: readonly(toasts),
    push,
    success,
    error,
    info,
    remove
  }
}
