<script lang="ts">
  import { onMount } from 'svelte'
  import { location } from 'svelte-spa-router'
  import { theme, THEMES, type Theme } from '../lib/stores/theme.js'
  import { locale } from '../lib/stores/locale.js'
  import { getStatus } from '../lib/api/client.js'
  import Toasts from './Toasts.svelte'
  import { wsConnectionState } from '../lib/stores/wsState.js'
  import * as m from '../lib/i18n/messages.js'

  $: currentPath = $location

  let appVersion = ''

  onMount(async () => {
    try {
      const status = await getStatus()
      appVersion = status.version
    } catch {
      // ignore - version just won't show
    }
  })

  function toggleLocale() {
    locale.set($locale === 'en' ? 'pt-BR' : 'en')
  }
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-900">
  <!-- Tabs Navigation -->
  <nav class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between">
        <div class="flex space-x-8">
          <a
            href="#/status"
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/status' || currentPath === '/'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            {m.nav_status()}
          </a>
          <a
            href="#/config"
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/config'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            {m.nav_config()}
          </a>
          <a
            href="#/logs"
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/logs'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            {m.nav_logs()}
          </a>
        </div>

        <div class="flex items-center gap-3">
          <!-- WebSocket connection indicator -->
          <div
            class="tooltip tooltip-bottom"
            data-tip={$wsConnectionState === 'connected' ? m.ws_connected() : $wsConnectionState === 'reconnecting' ? m.ws_reconnecting() : m.ws_disconnected()}
          >
            <span class="inline-block w-2 h-2 rounded-full {
              $wsConnectionState === 'connected' ? 'bg-success' :
              $wsConnectionState === 'reconnecting' ? 'bg-warning animate-pulse' :
              'bg-error'
            }"></span>
          </div>

          {#if appVersion}
            <span class="text-xs text-base-content/40">v{appVersion}</span>
          {/if}

          <!-- Language toggle -->
          <button
            on:click={toggleLocale}
            class="text-xs font-semibold px-2 py-1 rounded border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            title={$locale === 'en' ? 'Mudar para Português' : 'Switch to English'}
          >
            {$locale === 'en' ? 'PT' : 'EN'}
          </button>

          <!-- Theme selector -->
          <label for="theme-select" class="sr-only">{m.theme_light()}</label>
          <select
            id="theme-select"
            value={$theme}
            on:change={(e) => {
              theme.set(e.currentTarget.value as Theme)
            }}
            class="block rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:border-blue-500 focus:ring-blue-500 py-1 px-2"
          >
            <option value={THEMES.LIGHT}>{m.theme_light()}</option>
            <option value={THEMES.DARK}>{m.theme_dark()}</option>
            <option value={THEMES.SYSTEM}>{m.theme_system()}</option>
          </select>
        </div>
      </div>
    </div>
  </nav>

  <!-- Page Content — re-keyed on locale change to force re-render of all translations -->
  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    {#key $locale}
      <slot />
    {/key}
  </main>
</div>

<Toasts />
