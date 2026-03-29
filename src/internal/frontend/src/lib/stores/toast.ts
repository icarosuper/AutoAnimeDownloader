import { writable } from 'svelte/store'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface Toast {
  id: number
  type: ToastType
  message: string
}

let nextId = 0

const { subscribe, update } = writable<Toast[]>([])

function add(type: ToastType, message: string, duration = 4000): void {
  const id = nextId++
  update(toasts => [...toasts, { id, type, message }])
  setTimeout(() => remove(id), duration)
}

function remove(id: number): void {
  update(toasts => toasts.filter(t => t.id !== id))
}

export const toasts = { subscribe }

export const toast = {
  success: (message: string, duration?: number) => add('success', message, duration),
  error:   (message: string, duration?: number) => add('error',   message, duration),
  warning: (message: string, duration?: number) => add('warning', message, duration),
  info:    (message: string, duration?: number) => add('info',    message, duration),
}
