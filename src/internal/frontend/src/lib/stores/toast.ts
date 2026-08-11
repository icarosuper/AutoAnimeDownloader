import { writable } from 'svelte/store'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

/** Link opcional dentro do toast (ex.: "ver downloads" depois de enfileirar episódios). */
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

const { subscribe, update } = writable<Toast[]>([])

function add(type: ToastType, message: string, duration = 4000, link?: ToastLink): void {
  const id = nextId++
  update(toasts => [...toasts, { id, type, message, link }])
  setTimeout(() => remove(id), duration)
}

function remove(id: number): void {
  update(toasts => toasts.filter(t => t.id !== id))
}

export const toasts = { subscribe }

export const toast = {
  success: (message: string, duration?: number, link?: ToastLink) => add('success', message, duration, link),
  error:   (message: string, duration?: number) => add('error',   message, duration),
  warning: (message: string, duration?: number) => add('warning', message, duration),
  info:    (message: string, duration?: number) => add('info',    message, duration),
}
