import './app.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { theme } from './lib/stores/theme.js'

// Initialize theme store - it will apply the theme automatically
if (typeof window !== 'undefined') {
  // Subscribe to ensure the store is initialized and theme is applied
  theme.subscribe(() => {})
}

mount(App, { target: document.getElementById('app') || document.body })

