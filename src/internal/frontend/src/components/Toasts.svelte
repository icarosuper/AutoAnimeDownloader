<script lang="ts">
  import { toasts, type Toast } from '../lib/stores/toast.js'

  function alertClass(type: Toast['type']): string {
    switch (type) {
      case 'success': return 'alert-success'
      case 'error':   return 'alert-error'
      case 'warning': return 'alert-warning'
      case 'info':    return 'alert-info'
    }
  }

  function icon(type: Toast['type']): string {
    switch (type) {
      case 'success': return `<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>`
      case 'error':   return `<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>`
      case 'warning': return `<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>`
      case 'info':    return `<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>`
    }
  }
</script>

<div class="toast toast-end toast-bottom z-50 gap-2">
  {#each $toasts as t (t.id)}
    <div
      class="alert {alertClass(t.type)} shadow-lg max-w-sm animate-in"
      role="alert"
    >
      <svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        {@html icon(t.type)}
      </svg>
      <span class="text-sm">{t.message}</span>
    </div>
  {/each}
</div>

<style>
  .animate-in {
    animation: slide-in 0.2s ease-out;
  }

  @keyframes slide-in {
    from {
      opacity: 0;
      transform: translateX(1rem);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }
</style>
