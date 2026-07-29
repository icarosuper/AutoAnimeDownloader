<script lang="ts">
  // Modal — spec §6 (Fase 2). Base for the dialogs later screens build (this phase adds no
  // consumer — the existing `ConfirmDialog`/`TorrentDeleteDialog` are left untouched per the
  // brief). Focus-trapped, closes on Esc and on an outside (backdrop) click.
  //
  // Unlike `components/ConfirmDialog.svelte` (native `<dialog>` + daisyUI `.modal-open`), this
  // uses a plain `<div role="dialog">` so a screen composing arbitrary content (forms, lists)
  // isn't tied to daisyUI's modal chrome — the primitive only owns the overlay/focus-trap
  // mechanics, all styling of the content is the slot's job.
  import { createEventDispatcher, tick } from 'svelte'

  export let open = false
  /** id of the title element inside the slot, wired to aria-labelledby for screen readers. */
  export let labelledBy: string | undefined = undefined

  const dispatch = createEventDispatcher<{ close: void }>()

  let dialogEl: HTMLDivElement | undefined
  let previouslyFocused: HTMLElement | null = null

  function close() {
    dispatch('close')
  }

  function focusableEls(): HTMLElement[] {
    if (!dialogEl) return []
    return Array.from(
      dialogEl.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
      ),
    )
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!open) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      close()
      return
    }
    if (e.key === 'Tab') {
      const els = focusableEls()
      if (els.length === 0) return
      const first = els[0]
      const last = els[els.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  async function trapInitialFocus() {
    await tick()
    const els = focusableEls()
    ;(els[0] ?? dialogEl)?.focus()
  }

  // Opening: remember what had focus (so it can be restored) and move focus inside. Closing:
  // give focus back to whatever triggered the modal.
  $: if (open) {
    if (!previouslyFocused && typeof document !== 'undefined') {
      previouslyFocused = document.activeElement as HTMLElement
    }
    trapInitialFocus()
  } else if (previouslyFocused) {
    previouslyFocused.focus()
    previouslyFocused = null
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="absolute inset-0 bg-window/70" on:click={close} aria-hidden="true"></div>
    <div
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby={labelledBy}
      tabindex="-1"
      class="relative z-10 w-full max-w-lg rounded-modal border border-default bg-card p-6 shadow-elevation"
    >
      <slot />
    </div>
  </div>
{/if}
