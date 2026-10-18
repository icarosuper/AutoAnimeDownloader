<script lang="ts">
  // AppShell — rail + content grid (spec §5, Phase 1 of the UI redesign). Picks NavRail
  // (desktop) vs NavTabBar (mobile) at the 768px breakpoint and hosts <Toasts />. Replaces
  // components/Layout.svelte, which wrote the six nav links twice (desktop block + mobile
  // block) — the list now lives once in lib/navItems.ts.
  //
  // The rail-vs-tab-bar choice is made in JS (matchMedia), not with a pair of
  // `hidden md:flex` / `md:hidden` blocks always present in the DOM: NavRail and NavTabBar
  // each mount their own MoreMenu (which hosts <select id="theme-select-mobile"> on mobile,
  // and the rail has its own <select id="theme-select"> fixed in the footer) — with both
  // blocks always in the DOM, both would exist at once (only hidden via CSS), which would
  // duplicate those ids and leave two identical interactive elements for screen readers /
  // role tests. With the JS swap, only one of the two is mounted at a time.
  import { onDestroy, onMount } from 'svelte'
  import { location } from 'svelte-spa-router'
  import SystemBanner from './SystemBanner.svelte'
  import Toasts from '../Toasts.svelte'
  import NavRail from './NavRail.svelte'
  import NavTabBar from './NavTabBar.svelte'

  const DESKTOP_QUERY = '(min-width: 768px)' // = Tailwind `md:`, as the spec asks
  const isBrowser = typeof window !== 'undefined'
  const desktopMql = isBrowser ? window.matchMedia(DESKTOP_QUERY) : null

  let isDesktop = desktopMql ? desktopMql.matches : true

  function handleBreakpointChange(event: MediaQueryListEvent) {
    isDesktop = event.matches
  }

  onMount(() => {
    desktopMql?.addEventListener('change', handleBreakpointChange)
  })

  onDestroy(() => {
    desktopMql?.removeEventListener('change', handleBreakpointChange)
  })

  $: currentPath = $location
</script>

<div class="flex min-h-screen bg-window text-body">
  {#if isDesktop}
    <NavRail {currentPath} />
  {/if}

  <div class="flex min-w-0 flex-1 flex-col">
    <!-- System banner: above the content and outside <main>, because it talks about the
         whole page (the daemon is down, AniList is down), not about the current route. -->
    <SystemBanner />

    <!-- Page Content — no {#key $locale} here; each route handles its own reactivity -->
    <main class="mx-auto w-full max-w-7xl flex-1 px-4 pb-24 pt-8 sm:px-6 md:pb-8 lg:px-8">
      <slot />
    </main>
  </div>

  {#if !isDesktop}
    <NavTabBar {currentPath} />
  {/if}

  <Toasts />
</div>
