<script lang="ts">
  // NavRail — 92px vertical rail (spec §5 asks for 74px; widened together with the general
  // type-scale increase — see the note on `navItemClass`). Only mounted on desktop (>=768px);
  // AppShell decides that in JS (matchMedia), not via `hidden md:flex` classes, so the mobile
  // NavTabBar/MoreMenu never coexist in the DOM (would duplicate ids, e.g. #theme-select).
  import { onMount } from 'svelte'
  import { getStatus } from '../../lib/api/client.js'
  import {
    isMoreMenuActive,
    isNavItemActive,
    moreTriggerIcon,
    moreTriggerLabel,
    primaryNavItems,
    secondaryNavItems,
  } from '../../lib/navItems.js'
  import * as m from '../../lib/i18n/messages.js'
  import { activeTorrentCount } from '../../lib/stores/activeTorrents.js'
  import { locale } from '../../lib/stores/locale.js'
  import { theme, THEMES, type Theme } from '../../lib/stores/theme.js'
  import { wsConnectionState } from '../../lib/stores/wsState.js'
  import MoreMenu from './MoreMenu.svelte'

  export let currentPath = ''

  let moreOpen = false
  let appVersion = ''

  $: T = $locale && {
    themeLight: m.theme_light(),
    themeDark: m.theme_dark(),
    themeSystem: m.theme_system(),
    wsConnected: m.ws_connected(),
    wsReconnecting: m.ws_reconnecting(),
    wsDisconnected: m.ws_disconnected(),
    more: moreTriggerLabel(),
    primaryItems: primaryNavItems.map((item) => ({ ...item, label: item.label() })),
    secondaryItems: secondaryNavItems.map((item) => ({ ...item, label: item.label() })),
  }

  onMount(async () => {
    try {
      const status = await getStatus()
      appVersion = status.version
    } catch {
      // ignore — version is best-effort, same as the old Layout.svelte
    }
  })

  $: wsTooltip = T
    ? $wsConnectionState === 'connected'
      ? T.wsConnected
      : $wsConnectionState === 'reconnecting'
        ? T.wsReconnecting
        : T.wsDisconnected
    : ''

  function toggleLocale() {
    locale.set($locale === 'en' ? 'pt-BR' : 'en')
  }

  // The width (82px, inside a 92px rail) is dictated by the longest label: "Settings" in
  // Manrope 700/10.5px measures 75px. At the previous size (9.5px in a 56px item) that label
  // already leaked out of the rail; raising the font without widening the item would worsen the leak.
  function navItemClass(active: boolean): string {
    return `flex h-[48px] w-[82px] flex-col items-center justify-center gap-[5px] rounded-field text-[10.5px] transition-colors ${
      active ? 'bg-accent-tint/16 font-bold text-nav-active' : 'font-semibold text-subtle hover:text-body'
    }`
  }
</script>

<!-- `sticky top-0` + `h-screen`: the rail is viewport-tall, but without positioning it was
     just a flex item in normal flow — on a page taller than the screen it scrolled away and
     vanished. Sticky (not `fixed`) because the rail is the first item of AppShell's flex
     row: it has to keep occupying the 92px of width in flow, otherwise content would pass
     underneath.

     `z-30` (the same as NavTabBar) is NOT decorative: `position: sticky` always creates a
     stacking context, so the MoreMenu panel's `z-50` only orders things INSIDE the rail.
     Without a z-index here, the whole rail paints on the root's z-auto layer in tree order
     and loses to any positioned element that comes later in the DOM — that was daisyUI's
     `.card` (`position: relative`) on the Priorities screen, which appeared in front of the
     menu. Modal/Toasts/ActionMenu sit at z-50 on the root, so they stay above the rail. -->
<nav
  class="sticky top-0 z-30 flex h-screen w-[92px] shrink-0 flex-col border-r border-default bg-sunken"
  aria-label="Main"
>
  <div class="flex flex-col items-center gap-1 pt-4">
    {#if T}
      {#each T.primaryItems as item (item.id)}
        <a
          href="#{item.path}"
          class={navItemClass(isNavItemActive(item, currentPath))}
          aria-current={isNavItemActive(item, currentPath) ? 'page' : undefined}
        >
          <span class="relative">
            <svelte:component this={item.icon} size={18} strokeWidth={2} />
            {#if item.id === 'downloads' && $activeTorrentCount > 0}
              <span
                class="absolute -right-1.5 -top-1.5 flex h-[17px] min-w-[17px] items-center justify-center rounded-full bg-ok px-1 font-mono text-[10px] font-extrabold text-on-ok"
              >
                {$activeTorrentCount}
              </span>
            {/if}
          </span>
          {item.label}
        </a>
      {/each}

      <div class="my-1 w-6 border-t border-default" role="separator"></div>

      {#each T.secondaryItems as item (item.id)}
        <a
          href="#{item.path}"
          class={navItemClass(isNavItemActive(item, currentPath))}
          aria-current={isNavItemActive(item, currentPath) ? 'page' : undefined}
        >
          <svelte:component this={item.icon} size={18} strokeWidth={2} />
          {item.label}
        </a>
      {/each}

      <div class="relative">
        <button
          type="button"
          class={navItemClass(isMoreMenuActive(currentPath))}
          aria-haspopup="menu"
          aria-expanded={moreOpen}
          on:click={() => (moreOpen = !moreOpen)}
        >
          <svelte:component this={moreTriggerIcon} size={18} strokeWidth={2} />
          {T.more}
        </button>

        <MoreMenu bind:open={moreOpen} {currentPath} anchorClass="bottom-0 left-full ml-2" />
      </div>
    {/if}
  </div>

  <div class="mt-auto flex flex-col items-center gap-2 pb-4">
    <!-- the tooltip comes from the wrapper's `data-tip` (CSS in src/app.css, which replaced daisyUI's)
         API and would render as always-visible inline text here. -->
    <div class="tooltip tooltip-right" data-tip={wsTooltip}>
      <span
        class="inline-block h-2 w-2 rounded-full {$wsConnectionState === 'connected'
          ? 'bg-ok'
          : $wsConnectionState === 'reconnecting'
            ? 'animate-pulse bg-warn'
            : 'bg-danger'}"
      ></span>
    </div>

    {#if appVersion}
      <span class="text-[11px] text-subtle">v{appVersion}</span>
    {/if}

    <button
      type="button"
      on:click={toggleLocale}
      class="rounded-control border border-default px-1.5 py-0.5 text-[11px] font-semibold text-body hover:bg-control transition-colors"
      title={$locale === 'en' ? 'Switch to Português' : 'Mudar para English'}
    >
      {$locale === 'en' ? 'EN' : 'PT'}
    </button>

    {#if T}
      <label for="theme-select" class="sr-only">{T.themeLight}</label>
      <select
        id="theme-select"
        value={$theme}
        on:change={(e) => theme.set(e.currentTarget.value as Theme)}
        class="w-[80px] rounded-control border border-default bg-control text-heading outline-none transition-colors focus:border-accent px-1.5 py-1 text-[11px]"
      >
        <option value={THEMES.LIGHT}>{T.themeLight}</option>
        <option value={THEMES.DARK}>{T.themeDark}</option>
        <option value={THEMES.SYSTEM}>{T.themeSystem}</option>
      </select>
    {/if}
  </div>
</nav>
