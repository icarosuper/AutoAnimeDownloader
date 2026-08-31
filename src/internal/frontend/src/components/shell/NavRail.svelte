<script lang="ts">
  // NavRail — rail vertical de 92px (o spec §5 pede 74px; alargado junto com o aumento geral da
  // escala tipográfica — ver a nota em `navItemClass`). Só é montado no desktop (>=768px); AppShell
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

  // A largura (82px, dentro de um rail de 92px) é ditada pelo rótulo mais longo: "Configurações"
  // em Manrope 700/10.5px mede 75px. No tamanho anterior (9.5px num item de 56px) esse rótulo já
  // vazava para fora do rail; aumentar a fonte sem alargar o item pioraria o vazamento.
  function navItemClass(active: boolean): string {
    return `flex h-[48px] w-[82px] flex-col items-center justify-center gap-[5px] rounded-field text-[10.5px] transition-colors ${
      active ? 'bg-accent-tint/16 font-bold text-nav-active' : 'font-semibold text-subtle hover:text-body'
    }`
  }
</script>

<!-- `sticky top-0` + `h-screen`: o rail tem a altura da viewport, mas sem posicionamento ele
     era só um item de flex no fluxo normal — em página mais alta que a tela ele rolava junto e
     sumia. Sticky (e não `fixed`) porque o rail é o primeiro item do flex row do AppShell: ele
     precisa continuar ocupando os 92px de largura no fluxo, senão o conteúdo passaria por baixo.

     `z-30` (o mesmo do NavTabBar) NÃO é decorativo: `position: sticky` cria contexto de
     empilhamento sempre, então o `z-50` do painel do MoreMenu só ordena coisas DENTRO do rail.
     Sem z-index aqui, o rail inteiro pinta na camada z-auto da raiz em ordem de árvore e perde
     para qualquer elemento posicionado que venha depois no DOM — era o caso do `.card` do
     daisyUI (`position: relative`) na tela de Prioridades, que aparecia na frente do menu.
     Modal/Toasts/ActionMenu ficam em z-50 na raiz, ou seja, continuam acima do rail. -->
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
    <!-- o tooltip sai do `data-tip` do wrapper (CSS em src/app.css, que substituiu o do daisyUI)
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
