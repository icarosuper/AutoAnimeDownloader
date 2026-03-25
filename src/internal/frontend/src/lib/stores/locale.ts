import { writable } from 'svelte/store'
import { setLocale, locales } from '../i18n/runtime.js'

export type Locale = 'en' | 'pt-BR'

const LOCALE_KEY = 'PARAGLIDE_LOCALE'
const isBrowser = typeof window !== 'undefined'

function detectBrowserLocale(): Locale {
  if (!isBrowser) return 'en'
  const lang = navigator.language || ''
  if (lang.startsWith('pt')) return 'pt-BR'
  return 'en'
}

function getInitialLocale(): Locale {
  if (!isBrowser) return 'en'
  const stored = localStorage.getItem(LOCALE_KEY) as Locale | null
  if (stored && (locales as readonly string[]).includes(stored)) return stored
  return detectBrowserLocale()
}

const initial = getInitialLocale()
setLocale(initial, { reload: false })

const { subscribe, set } = writable<Locale>(initial)

export const locale = {
  subscribe,
  set: (newLocale: Locale) => {
    if (isBrowser) localStorage.setItem(LOCALE_KEY, newLocale)
    setLocale(newLocale, { reload: false })
    set(newLocale)
  }
}

export { locales }
