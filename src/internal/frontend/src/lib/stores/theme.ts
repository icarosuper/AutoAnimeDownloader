import { writable, type Writable } from 'svelte/store'

// Check if we're in browser environment
const isBrowser = typeof window !== 'undefined'

const THEME_KEY = 'theme-preference'

export const THEMES = {
  LIGHT: 'light',
  DARK: 'dark',
  SYSTEM: 'system'
} as const

export type Theme = typeof THEMES.LIGHT | typeof THEMES.DARK | typeof THEMES.SYSTEM

function getSystemTheme(): 'light' | 'dark' {
  if (!isBrowser) return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function getStoredTheme(): Theme {
  if (!isBrowser) return THEMES.SYSTEM
  return (localStorage.getItem(THEME_KEY) as Theme) || THEMES.SYSTEM
}

function applyTheme(themeValue: Theme): void {
  if (!isBrowser) return

  const root = document.documentElement
  const effectiveTheme = themeValue === THEMES.SYSTEM ? getSystemTheme() : themeValue

  if (effectiveTheme === 'dark') {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

interface ThemeStore extends Writable<Theme> {
  set: (value: Theme) => void
}

function createThemeStore(): ThemeStore {
  const storedTheme = getStoredTheme()
  const { subscribe, set, update } = writable<Theme>(storedTheme)

  // Apply initial theme
  if (isBrowser) {
    applyTheme(storedTheme)
  }

  // Listen for system theme changes
  if (isBrowser) {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const handleSystemThemeChange = () => {
      const currentTheme = (localStorage.getItem(THEME_KEY) as Theme) || THEMES.SYSTEM
      if (currentTheme === THEMES.SYSTEM) {
        applyTheme(THEMES.SYSTEM)
      }
    }
    mediaQuery.addEventListener('change', handleSystemThemeChange)
  }

  return {
    subscribe,
    set: (newTheme: Theme) => {
      if (isBrowser) {
        localStorage.setItem(THEME_KEY, newTheme)
        applyTheme(newTheme)
      }
      set(newTheme)
    },
    update
  }
}

export const theme = createThemeStore()

