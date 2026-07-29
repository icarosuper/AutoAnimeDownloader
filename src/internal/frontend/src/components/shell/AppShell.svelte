<script lang="ts">
  // AppShell — grid rail + conteúdo (spec §5, Fase 1 do redesign de UI). Decide NavRail
  // (desktop) vs NavTabBar (mobile) no breakpoint 768px e hospeda <Toasts />. Substitui
  // components/Layout.svelte, que escrevia os seis links de navegação duas vezes (bloco
  // desktop + bloco mobile) — a lista agora vive uma única vez em lib/navItems.ts.
  //
  // A escolha rail-vs-tab-bar é feita em JS (matchMedia), não com um par de blocos
  // `hidden md:flex` / `md:hidden` sempre presentes no DOM: NavRail e NavTabBar cada um monta
  // seu próprio MoreMenu (que hospeda o <select id="theme-select-mobile"> no mobile, e o rail
  // tem seu próprio <select id="theme-select"> fixo no rodapé) — com os dois blocos sempre no
  // DOM, ambos existiriam ao mesmo tempo (só escondidos via CSS), o que duplicaria esses ids e
  // deixaria dois elementos interativos idênticos para leitores de tela / testes de role. Com
  // a troca em JS, só um dos dois é montado por vez.
  import { onDestroy, onMount } from 'svelte'
  import { location } from 'svelte-spa-router'
  import Toasts from '../Toasts.svelte'
  import NavRail from './NavRail.svelte'
  import NavTabBar from './NavTabBar.svelte'

  const DESKTOP_QUERY = '(min-width: 768px)' // = Tailwind `md:`, conforme pedido pelo spec
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
