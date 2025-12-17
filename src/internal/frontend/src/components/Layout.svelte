<script lang="ts">
  import { link, location } from 'svelte-spa-router'
  import { theme, THEMES, type Theme } from '../lib/stores/theme.js'

  $: currentPath = $location
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-900">
  <!-- Tabs Navigation -->
  <nav class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between">
        <div class="flex space-x-8">
          <a
            href="/status"
            use:link
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/status' || currentPath === '/'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            Status
          </a>
          <a
            href="/episodes"
            use:link
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/episodes'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            Episodes
          </a>
          <a
            href="/config"
            use:link
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/config'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            Config
          </a>
          <a
            href="/logs"
            use:link
            class="inline-flex items-center px-1 pt-4 pb-4 border-b-2 text-sm font-medium transition-colors {currentPath === '/logs'
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-600'}"
          >
            Logs
          </a>
        </div>
        
        <!-- Theme Selector -->
        <div class="flex items-center">
          <label for="theme-select" class="sr-only">Theme</label>
          <select
            id="theme-select"
            value={$theme}
            on:change={(e) => {
              theme.set(e.currentTarget.value as Theme)
            }}
            class="block rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:border-blue-500 focus:ring-blue-500 py-1 px-2"
          >
            <option value={THEMES.LIGHT}>Light</option>
            <option value={THEMES.DARK}>Dark</option>
            <option value={THEMES.SYSTEM}>System</option>
          </select>
        </div>
      </div>
    </div>
  </nav>

  <!-- Page Content -->
  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <slot />
  </main>
</div>

