<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { STATUS_SLUGS, statusLabel } from "../lib/utils/torrentStatus.js";

  // Controlado pelo pai (Downloads.svelte é quem guarda o ViewState na URL); este componente
  // só busca busca/filtro/seleção e emite eventos — não faz fetch de dado nenhum.
  export let query = "";
  export let statuses: string[] = [];
  export let selectedCount = 0;
  export let bulkBusy = false;

  const dispatch = createEventDispatcher<{
    search: string;
    statusesChange: string[];
    bulkPause: void;
    bulkResume: void;
    bulkAnnounce: void;
    bulkDelete: void;
    deselectAll: void;
  }>();

  $: T = $locale && {
    searchPlaceholder: m.downloads_search_placeholder(),
    statusFilter: m.downloads_status_filter(),
    selected: m.downloads_selected({ count: selectedCount }),
    bulkPause: m.downloads_bulk_pause(),
    bulkResume: m.downloads_bulk_resume(),
    bulkAnnounce: m.downloads_bulk_announce(),
    bulkDelete: m.downloads_bulk_delete(),
    deselectAll: m.downloads_deselect_all(),
  };

  function onSearchInput(e: Event) {
    dispatch("search", (e.target as HTMLInputElement).value);
  }

  function toggleStatus(slug: string) {
    const next = statuses.includes(slug)
      ? statuses.filter((s) => s !== slug)
      : [...statuses, slug];
    dispatch("statusesChange", next);
  }
</script>

<div class="flex flex-wrap items-center gap-3">
  <input
    type="search"
    class="input input-sm input-bordered w-full sm:w-64"
    placeholder={T && T.searchPlaceholder}
    value={query}
    on:input={onSearchInput}
  />

  <div class="dropdown">
    <div tabindex="0" role="button" class="btn btn-sm btn-outline">
      {T && T.statusFilter}{statuses.length > 0 ? ` (${statuses.length})` : ""}
    </div>
    <ul class="dropdown-content menu bg-base-100 border border-base-300 rounded-box z-10 w-56 p-2 shadow">
      {#each STATUS_SLUGS as slug (slug)}
        <li>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              class="checkbox checkbox-xs"
              checked={statuses.includes(slug)}
              on:change={() => toggleStatus(slug)}
            />
            <span class="text-sm">{$locale && statusLabel(slug)}</span>
          </label>
        </li>
      {/each}
    </ul>
  </div>
</div>

{#if selectedCount > 0}
  <div class="flex flex-wrap items-center gap-2 bg-base-200 border border-base-300 rounded-lg px-3 py-2">
    <span class="text-sm font-medium text-base-content">{T && T.selected}</span>
    <button class="btn btn-xs" disabled={bulkBusy} on:click={() => dispatch("bulkPause")}>
      {T && T.bulkPause}
    </button>
    <button class="btn btn-xs" disabled={bulkBusy} on:click={() => dispatch("bulkResume")}>
      {T && T.bulkResume}
    </button>
    <button class="btn btn-xs btn-ghost" disabled={bulkBusy} on:click={() => dispatch("bulkAnnounce")}>
      {T && T.bulkAnnounce}
    </button>
    <button class="btn btn-xs btn-error" disabled={bulkBusy} on:click={() => dispatch("bulkDelete")}>
      {T && T.bulkDelete}
    </button>
    <button class="btn btn-xs btn-ghost ml-auto" on:click={() => dispatch("deselectAll")}>
      {T && T.deselectAll}
    </button>
  </div>
{/if}
