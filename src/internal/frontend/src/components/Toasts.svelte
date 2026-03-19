<script lang="ts">
  import { fly } from 'svelte/transition'
  import { toasts } from '../lib/stores/toasts.js'
</script>

<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none">
  {#each $toasts as toast (toast.id)}
    <div
      class="flex items-start gap-2 bg-white dark:bg-gray-800 border border-red-300 dark:border-red-700 rounded-lg shadow-lg px-4 py-3 max-w-xs pointer-events-auto"
      transition:fly={{ y: 20, duration: 200 }}
    >
      <svg
        class="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"
        />
      </svg>
      <p class="text-sm text-gray-800 dark:text-gray-200 flex-1 break-words">{toast.message}</p>
      <button
        on:click={() => toasts.remove(toast.id)}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 flex-shrink-0"
        aria-label="Fechar"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}
</div>
