<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import { Search, X } from "@lucide/svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import type { FilterPreset } from "../lib/utils/torrentFilters.js";
  import Button from "./ui/Button.svelte";
  import Checkbox from "./ui/Checkbox.svelte";

  // Controlado pelo pai (Downloads.svelte é quem guarda o ViewState na URL); este componente
  // só busca busca/filtro/seleção e emite eventos — não faz fetch de dado nenhum.
  //
  // Fase 5 (spec §9.3) trocou o dropdown de status por quatro pills com contagem e passou a
  // barra de seleção a ficar SEMPRE visível (esmaecida e desabilitada quando não há nada
  // selecionado), em vez de aparecer e sumir empurrando a lista.
  export let query = "";
  export let preset: FilterPreset | null = "all";
  export let counts = { all: 0, downloading: 0, seeding: 0, problems: 0 };
  export let selectedCount = 0;
  export let allSelected = false;
  export let someSelected = false;
  export let bulkBusy = false;

  const dispatch = createEventDispatcher<{
    search: string;
    presetChange: FilterPreset;
    selectAll: void;
    bulkPrioritize: void;
    bulkPause: void;
    bulkResume: void;
    bulkAnnounce: void;
    bulkDelete: void;
    deselectAll: void;
  }>();

  $: T = $locale && {
    searchPlaceholder: m.downloads_search_placeholder(),
    clearSearch: m.status_clear_search(),
    selectAll: m.downloads_select_all(),
    selected: m.downloads_selected({ count: selectedCount }),
    bulkPrioritize: m.downloads_bulk_prioritize(),
    bulkPause: m.downloads_bulk_pause(),
    bulkResume: m.downloads_bulk_resume(),
    bulkAnnounce: m.downloads_bulk_announce(),
    bulkDelete: m.downloads_bulk_delete(),
    deselectAll: m.downloads_deselect_all(),
  };

  // Declarado aqui, e não inline no `{#each}`, para o literal não perder o tipo da união
  // `FilterPreset` e virar `string` no handler de clique.
  $: pills = [
    { id: "all" as FilterPreset, label: $locale && m.downloads_filter_all(), count: counts.all },
    { id: "downloading" as FilterPreset, label: $locale && m.downloads_status_downloading(), count: counts.downloading },
    { id: "seeding" as FilterPreset, label: $locale && m.downloads_status_seeding(), count: counts.seeding },
    { id: "problems" as FilterPreset, label: $locale && m.downloads_filter_problems(), count: counts.problems },
  ];
</script>

<div class="flex flex-col gap-3 border-b border-divider p-4 sm:flex-row sm:items-center">
  <label class="flex w-full shrink-0 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-1.5 sm:w-64">
    <Search size={16} strokeWidth={2} class="shrink-0 text-subtle" />
    <input
      type="search"
      placeholder={(T && T.searchPlaceholder) || ""}
      value={query}
      on:input={(e) => dispatch("search", e.currentTarget.value)}
      class="w-full min-w-0 bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
    />
    {#if query}
      <!-- O tooltip sai do `data-tip` do wrapper (CSS em src/app.css, que substituiu o do daisyUI). -->
      <div class="tooltip tooltip-left shrink-0" data-tip={T && T.clearSearch}>
        <button
          type="button"
          class="flex text-subtle hover:text-body"
          aria-label={T && T.clearSearch}
          on:click={() => dispatch("search", "")}
        >
          <X size={14} strokeWidth={2} />
        </button>
      </div>
    {/if}
  </label>

  <!-- `flex-wrap`, não `overflow-x-auto`: as quatro pills existem para responder "tem algo ali?"
       de bate-pronto, e atrás de um scroll horizontal as últimas ("Seeding", "Problems") ficavam
       fora da vista em tela estreita. Elas quebram para a linha de baixo quando não couberem.
       Sem `min-w-0` aqui: o mínimo automático deste flex item é o min-content (a pill mais
       larga), que é exatamente o piso desejado — com min-w-0 uma pill voltaria a estourar. -->
  <div class="flex flex-wrap items-center gap-2">
    {#each pills as pill (pill.id)}
      <button
        type="button"
        aria-pressed={preset === pill.id}
        on:click={() => dispatch("presetChange", pill.id)}
        class="inline-flex shrink-0 items-center gap-1.5 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {preset ===
        pill.id
          ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
          : 'border-default bg-control text-subtle hover:text-body'}"
      >
        {pill.label}
        <span class="font-mono text-[12px] opacity-70">{pill.count}</span>
      </button>
    {/each}
  </div>
</div>

<div class="flex flex-wrap items-center gap-2 border-b border-divider px-4 py-2.5">
  <Checkbox
    checked={allSelected}
    indeterminate={someSelected}
    label={(T && T.selectAll) || ""}
    on:change={() => dispatch("selectAll")}
  />

  <div
    class="ml-auto flex flex-wrap items-center gap-2 transition-opacity {selectedCount === 0 ? 'opacity-45' : ''}"
  >
    {#if selectedCount > 0}
      <span class="text-copy text-accent">{T && T.selected}</span>
    {/if}
    <Button variant="ghost" disabled={bulkBusy || selectedCount === 0} on:click={() => dispatch("bulkPrioritize")}>
      {T && T.bulkPrioritize}
    </Button>
    <Button variant="ghost" disabled={bulkBusy || selectedCount === 0} on:click={() => dispatch("bulkPause")}>
      {T && T.bulkPause}
    </Button>
    <Button variant="ghost" disabled={bulkBusy || selectedCount === 0} on:click={() => dispatch("bulkResume")}>
      {T && T.bulkResume}
    </Button>
    <Button variant="ghost" disabled={bulkBusy || selectedCount === 0} on:click={() => dispatch("bulkAnnounce")}>
      {T && T.bulkAnnounce}
    </Button>
    <Button variant="warn" disabled={bulkBusy || selectedCount === 0} on:click={() => dispatch("bulkDelete")}>
      {T && T.bulkDelete}
    </Button>
    {#if selectedCount > 0}
      <Button variant="ghost" on:click={() => dispatch("deselectAll")}>{T && T.deselectAll}</Button>
    {/if}
  </div>
</div>
