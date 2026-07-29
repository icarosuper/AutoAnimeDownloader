<script context="module" lang="ts">
  // ActionMenu — spec §6 (Fase 2). One item definition renders as a desktop dropdown
  // (min-width 236px, 12px radius, --bg-menu background, flips upward when it would overflow
  // the viewport) OR a mobile action sheet (20px 20px 0 0 radius, 38x4px drag handle) — the
  // caller never picks which; this component decides from viewport width, same 768px
  // breakpoint AppShell uses for NavRail vs. NavTabBar.
  export interface ActionMenuItem {
    id: string
    label: string
    destructive?: boolean
    disabled?: boolean
  }

  // Enforces "only one ActionMenu open at a time" (spec §6/§11) across every instance mounted
  // in the app. Each instance registers its own `close` callback when it opens; opening a new
  // one closes whichever instance was previously registered. Module-scoped by design — the
  // constraint is app-wide, not per-parent-component.
  let openInstanceClose: (() => void) | null = null

  function registerOpen(close: () => void) {
    if (openInstanceClose && openInstanceClose !== close) {
      openInstanceClose()
    }
    openInstanceClose = close
  }

  function unregisterOpen(close: () => void) {
    if (openInstanceClose === close) {
      openInstanceClose = null
    }
  }
</script>

<script lang="ts">
  import { createEventDispatcher, onDestroy, tick } from 'svelte'

  export let items: ActionMenuItem[] = []
  /** Accessible name for the "..." trigger button — caller supplies translated text. */
  export let triggerLabel: string
  export let open = false

  const dispatch = createEventDispatcher<{ select: string }>()

  const MOBILE_QUERY = '(max-width: 767px)'
  const mql = typeof window !== 'undefined' ? window.matchMedia(MOBILE_QUERY) : null

  let rootEl: HTMLDivElement | undefined
  let panelEl: HTMLDivElement | undefined
  let isMobile = mql ? mql.matches : false
  let flipUp = false

  function syncViewport() {
    isMobile = mql ? mql.matches : false
  }

  function close() {
    open = false
    flipUp = false
  }

  async function openMenu() {
    open = true
    registerOpen(close)
    await tick()
    if (!isMobile && panelEl) {
      flipUp = panelEl.getBoundingClientRect().bottom > window.innerHeight
    }
  }

  function toggle() {
    if (open) close()
    else openMenu()
  }

  function selectItem(item: ActionMenuItem) {
    if (item.disabled) return
    dispatch('select', item.id)
    close()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      e.stopPropagation()
      close()
    }
  }

  function handleWindowClick(e: MouseEvent) {
    if (open && rootEl && !rootEl.contains(e.target as Node)) {
      close()
    }
  }

  onDestroy(() => unregisterOpen(close))
</script>

<svelte:window on:keydown={handleKeydown} on:click={handleWindowClick} on:resize={syncViewport} />

<div class="relative inline-block" bind:this={rootEl}>
  <button
    type="button"
    class="rounded-control p-1.5 text-subtle transition-colors hover:bg-control hover:text-body"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={triggerLabel}
    on:click|stopPropagation={toggle}
  >
    <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" aria-hidden="true">
      <circle cx="5" cy="12" r="2" />
      <circle cx="12" cy="12" r="2" />
      <circle cx="19" cy="12" r="2" />
    </svg>
  </button>

  {#if open}
    {#if isMobile}
      <div class="fixed inset-0 z-50 bg-window/70" aria-hidden="true"></div>
      <div
        bind:this={panelEl}
        role="menu"
        aria-label={triggerLabel}
        class="fixed inset-x-0 bottom-0 z-50 rounded-t-[20px] bg-menu pb-4 pt-2 shadow-elevation"
      >
        <div class="mx-auto mb-2 h-1 w-[38px] rounded-pill bg-default" aria-hidden="true"></div>
        {#each items as item (item.id)}
          <button
            type="button"
            role="menuitem"
            disabled={item.disabled}
            class="flex w-full items-center px-4 py-3 text-left text-copy transition-colors disabled:opacity-50 {item.destructive
              ? 'text-danger'
              : 'text-body'}"
            on:click|stopPropagation={() => selectItem(item)}
          >
            {item.label}
          </button>
        {/each}
      </div>
    {:else}
      <div
        bind:this={panelEl}
        role="menu"
        aria-label={triggerLabel}
        class="absolute right-0 z-50 min-w-[236px] rounded-field border border-default bg-menu py-2 shadow-elevation {flipUp
          ? 'bottom-full mb-2'
          : 'top-full mt-2'}"
      >
        {#each items as item (item.id)}
          <button
            type="button"
            role="menuitem"
            disabled={item.disabled}
            class="flex w-full items-center px-3 py-2 text-left text-copy transition-colors hover:bg-control disabled:opacity-50 {item.destructive
              ? 'text-danger'
              : 'text-body'}"
            on:click|stopPropagation={() => selectItem(item)}
          >
            {item.label}
          </button>
        {/each}
      </div>
    {/if}
  {/if}
</div>
