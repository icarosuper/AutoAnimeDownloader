<script lang="ts">
  // NavTabBar — bottom tab bar (spec §5): Status, Add anime, Downloads, Config, More. Only
  // mounted on mobile (<768px); see the comment in AppShell.svelte for why the rail-vs-tab-bar
  // choice is made in JS and not with `md:hidden`/`hidden md:flex` classes.
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

  // Top group of the rail (Status, Add anime, Downloads) + the bottom one (Config) = the 4
  // first columns; "More" is the 5th. Same source (lib/navItems.ts) NavRail uses — the tab
  // bar follows the list, never a copy of it.
  $: tabItems = $locale && [...primaryNavItems, ...secondaryNavItems].map((item) => ({
    ...item,
    label: item.label(),
  }))

  $: moreLabel = $locale && moreTriggerLabel()

  // `min-w-0` + the `truncate` on the label (see markup) keep a column from invading its
  // neighbor, and that's what lets the column count be a consequence of `navItems.ts` instead
  // of a fixed number here: columns are `flex-1`, so adding an item squeezes them instead of
  // wrapping the row. With 5 columns ~72px remain per column at 375px and ~61px at 320px —
  // labels like "Settings" (~82px at this scale) become ellipses, which is the correct degrade.
  function tabItemClass(active: boolean): string {
    return `flex min-h-[44px] min-w-0 flex-col items-center justify-center gap-[5px] rounded-field px-0.5 text-[11.5px] ${
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
              class="absolute -right-1.5 -top-1.5 flex h-[17px] min-w-[17px] items-center justify-center rounded-full bg-ok px-1 font-mono text-[10px] font-extrabold text-on-ok"
            >
              {$activeTorrentCount}
            </span>
          {/if}
        </span>
        <span class="w-full truncate text-center">{item.label}</span>
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
        <span class="w-full truncate text-center">{moreLabel}</span>
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
