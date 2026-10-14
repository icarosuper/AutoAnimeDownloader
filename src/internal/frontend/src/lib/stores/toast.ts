import { get, writable } from 'svelte/store'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

/** Optional link inside the toast (e.g. "see downloads" after queueing episodes). */
export interface ToastLink {
  href: string
  label: string
}

export interface Toast {
  id: number
  type: ToastType
  message: string
  link?: ToastLink
}

let nextId = 0

const store = writable<Toast[]>([])

function add(type: ToastType, message: string, duration = 4000, link?: ToastLink): void {
  // The API client toasts every failure on its own, and most pages also toast the same err.message
  // in their catch. The first one wins: it's the client's, and it carries the Report link.
  if (get(store).some(t => t.type === type && t.message === message)) return
  const id = nextId++
  store.update(toasts => [...toasts, { id, type, message, link }])
  setTimeout(() => remove(id), duration)
}

function remove(id: number): void {
  store.update(toasts => toasts.filter(t => t.id !== id))
}

export const toasts = { subscribe: store.subscribe }

export const toast = {
  success: (message: string, duration?: number, link?: ToastLink) => add('success', message, duration, link),
  error:   (message: string, duration?: number, link?: ToastLink) => add('error',   message, duration, link),
  warning: (message: string, duration?: number) => add('warning', message, duration),
  info:    (message: string, duration?: number) => add('info',    message, duration),
}
