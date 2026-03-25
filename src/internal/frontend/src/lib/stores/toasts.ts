import { writable } from 'svelte/store'

export type ToastType = 'error' | 'success' | 'info'

export interface Toast {
  id: number
  message: string
  type: ToastType
}

let nextId = 0

function createToastsStore() {
  const { subscribe, update } = writable<Toast[]>([])

  function add(message: string, type: ToastType = 'error', duration = 5000) {
    const id = nextId++
    update((toasts) => [...toasts, { id, message, type }])
    setTimeout(() => remove(id), duration)
  }

  function remove(id: number) {
    update((toasts) => toasts.filter((t) => t.id !== id))
  }

  return { subscribe, add, remove }
}

export const toasts = createToastsStore()
