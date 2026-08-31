<script lang="ts">
  // Empilhamento de toasts. Antes montava sobre `.toast`/`.alert` do daisyUI; agora a caixa é
  // a mesma superfície elevada que ActionMenu/MoreMenu usam (`bg-menu` + `shadow-elevation`),
  // com a cor de status apenas na borda e no ícone. Fundo sólido de propósito: o toast flutua
  // sobre a lista, e o `bg-*-tint/12` do Chip deixaria o conteúdo de baixo vazar através dele.
  import { toasts, type Toast } from '../lib/stores/toast.js'

  // Mesmo vocabulário de status de ui/Chip.svelte.
  const VARIANT_CLASSES: Record<Toast['type'], string> = {
    success: 'border-ok-tint/28 text-ok',
    error: 'border-danger-tint/28 text-danger',
    warning: 'border-warn-tint/28 text-warn',
    info: 'border-accent-tint/28 text-accent',
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

<div class="fixed bottom-4 right-4 z-50 flex flex-col items-end gap-2">
  {#each $toasts as t (t.id)}
    <div
      class="animate-in flex max-w-sm items-start gap-2 whitespace-normal rounded-field border bg-menu px-4 py-3 shadow-elevation {VARIANT_CLASSES[t.type]}"
      role="alert"
    >
      <svg class="mt-0.5 h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        {@html icon(t.type)}
      </svg>
      <span class="text-copy text-heading">{t.message}</span>
      {#if t.link}
        <a href={t.link.href} class="text-copy font-semibold underline">{t.link.label}</a>
      {/if}
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
