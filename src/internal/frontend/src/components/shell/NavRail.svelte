<script lang="ts">
  // NavRail — rail vertical de 74px (spec §5). Só é montado no desktop (>=768px); AppShell
  // decide isso em JS (matchMedia), não via classes `hidden md:flex`, para nunca ter o
  // NavTabBar/MoreMenu mobile coexistindo no DOM (evitaria ids duplicados, ex. #theme-select).
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
      // ignore — versão é best-effort, igual ao Layout.svelte antigo
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

  function navItemClass(active: boolean): string {
    return `flex h-[46px] w-14 flex-col items-center justify-center gap-[5px] rounded-field text-[9.5px] transition-colors ${
      active ? 'bg-accent-tint/16 font-bold text-nav-active' : 'font-semibold text-subtle hover:text-body'
    }`
  }
</script>

<nav
  class="flex h-screen w-[74px] shrink-0 flex-col border-r border-default bg-sunken"
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
                class="absolute -right-1.5 -top-1.5 flex h-[15px] w-[15px] items-center justify-center rounded-full bg-ok font-mono text-[9px] font-extrabold text-on-ok"
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
    <!-- daisyUI 4 renders the tooltip from data-tip; the .tooltip-content element is a v5-only
         API and would render as always-visible inline text here. -->
    <div class="tooltip tooltip-right" data-tip={wsTooltip}>
      <span
        class="inline-block h-2 w-2 rounded-full {$wsConnectionState === 'connected'
          ? 'bg-success'
          : $wsConnectionState === 'reconnecting'
            ? 'animate-pulse bg-warning'
            : 'bg-error'}"
      ></span>
    </div>

    {#if appVersion}
      <span class="text-[10px] text-subtle">v{appVersion}</span>
    {/if}

    <button
      type="button"
      on:click={toggleLocale}
      class="rounded-control border border-default px-1.5 py-0.5 text-[10px] font-semibold text-body hover:bg-control transition-colors"
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
        class="select select-bordered select-xs w-14 px-1 text-[10px]"
      >
        <option value={THEMES.LIGHT}>{T.themeLight}</option>
        <option value={THEMES.DARK}>{T.themeDark}</option>
        <option value={THEMES.SYSTEM}>{T.themeSystem}</option>
      </select>
    {/if}
  </div>
</nav>
