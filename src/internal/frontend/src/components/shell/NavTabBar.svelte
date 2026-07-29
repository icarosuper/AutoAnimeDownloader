<script lang="ts">
  // NavTabBar — tab bar inferior de 4 colunas (spec §5): Status, Downloads, Config, Mais. Só é
  // montada no mobile (<768px); ver o comentário em AppShell.svelte sobre por que a escolha
  // rail-vs-tab-bar é feita em JS e não com classes `md:hidden`/`hidden md:flex`.
  import {
    isMoreMenuActive,
    isNavItemActive,
    moreTriggerIcon,
    moreTriggerLabel,
    primaryNavItems,
    secondaryNavItems,
  } from '../../lib/navItems.js'
  import { activeTorrentCount } from '../../lib/stores/activeTorrents.js'
  import { locale } from '../../lib/stores/locale.js'
  import MoreMenu from './MoreMenu.svelte'

  export let currentPath = ''

  let moreOpen = false

  // Status + Downloads (grupo de cima do rail) + Config (grupo de baixo) = as 3 primeiras
  // colunas; "Mais" é a 4a. Mesma fonte (lib/navItems.ts) que o NavRail usa.
  $: tabItems = $locale && [...primaryNavItems, ...secondaryNavItems].map((item) => ({
    ...item,
    label: item.label(),
  }))

  $: moreLabel = $locale && moreTriggerLabel()

  function tabItemClass(active: boolean): string {
    return `flex min-h-[44px] flex-col items-center justify-center gap-[5px] rounded-field text-[10.5px] ${
      active ? 'bg-accent-tint/16 font-bold text-nav-active' : 'font-semibold text-subtle'
    }`
  }
</script>

<nav
  class="fixed inset-x-0 bottom-0 z-30 flex items-stretch border-t border-default bg-sunken pb-5.5 pt-2.5 px-2"
  aria-label="Main"
>
  {#if tabItems}
    {#each tabItems as item (item.id)}
      <a
        href="#{item.path}"
        class={`flex-1 ${tabItemClass(isNavItemActive(item, currentPath))}`}
        aria-current={isNavItemActive(item, currentPath) ? 'page' : undefined}
      >
        <span class="relative">
          <svelte:component this={item.icon} size={20} strokeWidth={2} />
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

    <div class="relative flex-1">
      <button
        type="button"
        class={`w-full h-full ${tabItemClass(isMoreMenuActive(currentPath))}`}
        aria-haspopup="menu"
        aria-expanded={moreOpen}
        on:click={() => (moreOpen = !moreOpen)}
      >
        <svelte:component this={moreTriggerIcon} size={20} strokeWidth={2} />
        {moreLabel}
      </button>

      <MoreMenu
        bind:open={moreOpen}
        {currentPath}
        anchorClass="bottom-full right-0 mb-2"
        showFooterControls
      />
    </div>
  {/if}
</nav>
