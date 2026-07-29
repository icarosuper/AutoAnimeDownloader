<script lang="ts">
  // MoreMenu — item "Mais" do NavRail (desktop) e da NavTabBar (mobile). Um componente só, com
  // dois contextos de uso (spec §5 + ambiguidade #3 do brief da Fase 1): sempre lista
  // Notificações/Prioridades/Logs (`moreMenuItems` de lib/navItems.ts, fonte única); no mobile
  // também hospeda tema/idioma/WS/versão, que no desktop já ficam fixos no rodapé do NavRail
  // (por isso `showFooterControls`).
  //
  // Implementação local e simples (fechamento por backdrop + Escape) porque o primitivo
  // genérico de menu é da Fase 2 (`components/ui/ActionMenu`) — não deste escopo.
  import { getStatus } from '../../lib/api/client.js'
  import { isNavItemActive, moreMenuItems } from '../../lib/navItems.js'
  import * as m from '../../lib/i18n/messages.js'
  import { locale } from '../../lib/stores/locale.js'
  import { theme, THEMES, type Theme } from '../../lib/stores/theme.js'
  import { wsConnectionState } from '../../lib/stores/wsState.js'

  export let open = false
  export let currentPath = ''
  /** Classes de posicionamento do painel — cada chamador (rail vs tab bar) ancora diferente. */
  export let anchorClass = ''
  /** Só true na tab bar mobile: ali o menu também hospeda tema/idioma/WS/versão. */
  export let showFooterControls = false

  let appVersion = ''

  $: T = $locale && {
    themeLight: m.theme_light(),
    themeDark: m.theme_dark(),
    themeSystem: m.theme_system(),
    wsConnected: m.ws_connected(),
    wsReconnecting: m.ws_reconnecting(),
    wsDisconnected: m.ws_disconnected(),
    items: moreMenuItems.map((item) => ({ ...item, label: item.label() })),
  }

  // Fecha ao navegar — equivalente ao antigo `$: if (currentPath) mobileMenuOpen = false` do
  // Layout.svelte.
  $: if (currentPath) open = false

  $: wsTooltip = T
    ? $wsConnectionState === 'connected'
      ? T.wsConnected
      : $wsConnectionState === 'reconnecting'
        ? T.wsReconnecting
        : T.wsDisconnected
    : ''

  $: if (open && showFooterControls && !appVersion) {
    getStatus()
      .then((status) => {
        appVersion = status.version
      })
      .catch(() => {
        // ignore — versão é best-effort, igual ao Layout.svelte antigo
      })
  }

  function toggleLocale() {
    locale.set($locale === 'en' ? 'pt-BR' : 'en')
  }

  function close() {
    open = false
  }

  function handleKeydown(event: KeyboardEvent) {
    if (open && event.key === 'Escape') close()
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <button
    type="button"
    class="fixed inset-0 z-40 cursor-default"
    tabindex="-1"
    aria-label="Close menu"
    on:click={close}
  ></button>

  <div
    role="menu"
    class="absolute z-50 min-w-[190px] rounded-field border border-default bg-menu py-2 shadow-elevation {anchorClass}"
  >
    {#if T}
      {#each T.items as item (item.id)}
        <a
          role="menuitem"
          href="#{item.path}"
          class="flex min-h-[44px] items-center gap-2.5 px-3 py-2 text-copy transition-colors {isNavItemActive(
            item,
            currentPath,
          )
            ? 'bg-accent-tint/16 font-bold text-nav-active'
            : 'text-body hover:bg-control'}"
        >
          <svelte:component this={item.icon} size={18} strokeWidth={2} />
          {item.label}
        </a>
      {/each}
    {/if}

    {#if showFooterControls && T}
      <div class="mt-2 flex items-center justify-between gap-2 border-t border-divider px-3 pt-3">
        <div class="tooltip tooltip-top" data-tip={wsTooltip}>
          <!-- daisyUI 4 renders the tooltip from data-tip; the .tooltip-content element is a
               v5-only API and would render as always-visible inline text here. -->
          <span
            class="inline-block h-2 w-2 rounded-full {$wsConnectionState === 'connected'
              ? 'bg-success'
              : $wsConnectionState === 'reconnecting'
                ? 'animate-pulse bg-warning'
                : 'bg-error'}"
          ></span>
        </div>

        {#if appVersion}
          <span class="text-[11px] text-subtle">v{appVersion}</span>
        {/if}

        <button
          type="button"
          on:click={toggleLocale}
          class="min-h-[44px] rounded-control border border-default px-2 py-1 text-caption font-semibold text-body hover:bg-control transition-colors"
          title={$locale === 'en' ? 'Switch to Português' : 'Mudar para English'}
        >
          {$locale === 'en' ? 'EN' : 'PT'}
        </button>
      </div>

      <div class="px-3 pt-2">
        <label for="theme-select-mobile" class="sr-only">{T.themeLight}</label>
        <select
          id="theme-select-mobile"
          value={$theme}
          on:change={(e) => theme.set(e.currentTarget.value as Theme)}
          class="select select-bordered select-sm w-full"
        >
          <option value={THEMES.LIGHT}>{T.themeLight}</option>
          <option value={THEMES.DARK}>{T.themeDark}</option>
          <option value={THEMES.SYSTEM}>{T.themeSystem}</option>
        </select>
      </div>
    {/if}
  </div>
{/if}
