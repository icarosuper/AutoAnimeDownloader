<script lang="ts">
  // AddAnime — screen for adding a standalone anime (spec "Standalone anime download", §Frontend).
  //
  // A route, not a modal: it is linkable, reloadable, and a cover grid fits better on mobile
  // than a modal. Management of already-existing standalones lives on Status, so this screen
  // has one job — find an anime and start tracking it.
  import { onDestroy, onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import { ArrowRight, ExternalLink, Search } from "@lucide/svelte";
  import {
    searchAniList,
    addStandaloneAnime,
    getTorrentCandidates,
    getConfig,
    ApiError,
    type AniListSearchResult,
    type BlockReason,
  } from "../lib/api/client.js";
  import Button from "../components/ui/Button.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import Loading from "../components/Loading.svelte";
  import Toggle from "../components/ui/Toggle.svelte";
  import { toast } from "../lib/stores/toast.js";
  import { pickerCache } from "../lib/stores/pickerCache.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // Search fires on submit (button or Enter), NOT on every keystroke: AniList's limit is 30
  // req/min per IP and the search is PriorityDisposable, so anyone typing slowly burned the
  // bucket in one request per key and saw the next search itself refused by the budget gate.
  // One submit = one request. AbortController stays because two submits in a row still race
  // each other, and the old response must not paint over the new one.
  const MIN_TERM = 3;
  const ANILIST_URL = "https://anilist.co/anime/";

  let term = "";
  /** Off by default: anyone who lands here wants to download something now, and what hasn't premiered doesn't download. */
  let includeUnreleased = false;
  let results: AniListSearchResult[] = [];
  let searching = false;
  let searchFailed = false;
  let searched = false;
  /** media ids in flight or already added this session — the card button leaves the "Add" state. */
  let adding = new Set<number>();
  let added = new Set<number>();
  let libraryConfigured = true;

  let inFlight: AbortController | undefined;

  onMount(async () => {
    try {
      const config = await getConfig();
      libraryConfigured = Boolean(config.completed_anime_path);
    } catch {
      // Without the config we can't claim the library is missing; the POST refuses with
      // LIBRARY_NOT_CONFIGURED either way.
    }
  });

  onDestroy(() => inFlight?.abort());

  // The toggle only re-runs the search when one is already on screen: it changes the filter
  // of a visible result. With no search yet, it only picks the parameter of the next submit.
  function onToggleUnreleased(): void {
    if (searched) runSearch();
  }

  async function runSearch(): Promise<void> {
    const q = term.trim();
    inFlight?.abort();

    if (q.length < MIN_TERM) {
      results = [];
      searched = false;
      searchFailed = false;
      searching = false;
      return;
    }

    const controller = new AbortController();
    inFlight = controller;
    searching = true;
    searchFailed = false;

    try {
      const found = await searchAniList(q, includeUnreleased, controller.signal);
      if (controller.signal.aborted) return;
      results = found;
      searched = true;
    } catch (error) {
      if (controller.signal.aborted || (error instanceof Error && error.name === "AbortError")) return;
      searchFailed = true;
      results = [];
    } finally {
      if (!controller.signal.aborted) searching = false;
    }
  }

  // The front is best-effort and the backend is the authority: block_reason comes from a
  // snapshot up to 60s old, so a card can be clickable for an anime that just entered the
  // list. The 409 translated below is the last word; there is no retry or revalidation, the
  // next search already comes correct.
  //
  // First search (no q) decides the route: empty items POSTs today's add and stays; a
  // non-empty list or NYAA_UNAVAILABLE opens the picker without saving; any other GET
  // error toasts and stays. `adding` is always cleared in finally so a return from the
  // picker is not stuck on the in-flight button.
  async function add(result: AniListSearchResult): Promise<void> {
    adding = new Set(adding).add(result.id);
    try {
      const { query, items } = await getTorrentCandidates(result.id);
      if (items.length === 0) {
        const { added: count } = await addStandaloneAnime(result.id);
        added = new Set(added).add(result.id);
        if (count > 0) {
          toast.success(m.add_toast_added({ count }), undefined, {
            href: "#/downloads",
            label: m.add_toast_downloads_link(),
          });
        } else {
          // A dry "0 episodes added" would read as failure in the most common case: an anime
          // that hasn't premiered yet.
          toast.info(m.add_toast_tracked_no_episodes());
        }
        return;
      }
      pickerCache.set({ mediaId: result.id, query, items, error: null });
      await push("/add/" + result.id + "/torrents");
    } catch (error) {
      if (error instanceof ApiError && error.code === "NYAA_UNAVAILABLE") {
        pickerCache.set({ mediaId: result.id, query: "", items: [], error });
        await push("/add/" + result.id + "/torrents");
        return;
      }
      toast.error(addErrorMessage(error));
    } finally {
      const next = new Set(adding);
      next.delete(result.id);
      adding = next;
    }
  }

  function addErrorMessage(error: unknown): string {
    if (!(error instanceof ApiError)) {
      return error instanceof Error ? error.message : m.add_search_error();
    }
    switch (error.code) {
      case "LIBRARY_NOT_CONFIGURED":
        return m.block_library_not_configured();
      case "ALREADY_BLACKLISTED":
        return m.block_blacklist();
      case "ALREADY_TRACKED":
        return m.block_tracked();
      case "ALREADY_DOWNLOADED":
        return m.block_downloaded();
      case "ALREADY_STANDALONE":
        return m.block_standalone();
      default:
        return error.message;
    }
  }

  function blockLabel(reason: BlockReason): string {
    switch (reason) {
      case "blacklist":
        return m.block_blacklist();
      case "tracked":
        return m.block_tracked();
      case "downloaded":
        return m.block_downloaded();
      default:
        return "";
    }
  }

  function metaLine(result: AniListSearchResult): string {
    const parts: string[] = [];
    if (result.format) parts.push(result.format.replace(/_/g, " "));
    if (result.year) parts.push(String(result.year));
    if (result.episodes) parts.push(m.add_meta_episodes({ count: result.episodes }));
    if (result.status) parts.push(result.status.replace(/_/g, " "));
    return parts.join(" · ");
  }

  // A card only ends in a dead button when there is nowhere to go. "standalone", "tracked"
  // and "downloaded" mean the anime ALREADY EXISTS in the app — and an anime that exists has
  // a page at #/status/{id}, because anime_id is AniList's own media id. On those three the
  // button becomes the link there; the reason, which used to live only in a tooltip
  // (invisible on mobile), becomes a line on the card.
  //
  // "blacklist" is the only exception: it's on an excluded list, the daemon doesn't process
  // it and there is no detail to open. Only it stays dimmed with the button disabled.
  $: cards =
    $locale &&
    results.map((result) => {
      const isAdded = added.has(result.id) || result.block_reason === "standalone";
      const blacklisted = result.block_reason === "blacklist";
      const hasStatusPage =
        isAdded || result.block_reason === "tracked" || result.block_reason === "downloaded";
      return {
        result,
        meta: metaLine(result),
        note: isAdded ? m.block_standalone() : blockLabel(result.block_reason),
        isAdding: adding.has(result.id),
        blacklisted,
        statusHref: hasStatusPage ? `#/status/${result.id}` : undefined,
        anilistHref: `${ANILIST_URL}${result.id}`,
      };
    });
</script>

<div class="space-y-4.5">
  <div>
    <h1 class="text-screen-title text-heading">{$locale && m.add_title()}</h1>
    <p class="mt-0.5 text-caption text-subtle">{$locale && m.add_subtitle()}</p>
  </div>

  {#if !libraryConfigured}
    <div
      role="alert"
      class="flex flex-wrap items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
    >
      {$locale && m.add_library_required()}
      <a href="#/config" class="underline">{$locale && m.nav_config()}</a>
    </div>
  {/if}

  <!-- <form>, not an on:keydown on the input: the Enter that fires submit is native form
       behavior, and the button takes part in it just by being type="submit". -->
  <form class="flex items-center gap-2" on:submit|preventDefault={runSearch}>
    <label
      class="flex flex-1 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-2 focus-within:border-accent"
    >
      <Search size={15} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
      <input
        type="search"
        bind:value={term}
        placeholder={$locale && m.add_search_placeholder()}
        aria-label={$locale && m.add_search_placeholder()}
        class="w-full bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
      />
    </label>
    <Button type="submit" disabled={term.trim().length < MIN_TERM || searching}>
      {$locale && m.add_btn_search()}
    </Button>
  </form>

  <Toggle
    id="add-include-unreleased"
    bind:checked={includeUnreleased}
    on:change={onToggleUnreleased}
    label={($locale && m.add_toggle_unreleased()) || ""}
  />

  {#if searching}
    <Loading message={$locale && m.add_search_placeholder()} />
  {:else if searchFailed}
    <p role="alert" class="text-copy text-danger">{$locale && m.add_search_error()}</p>
  {:else if !searched}
    <p class="text-copy text-subtle">{$locale && m.add_search_hint()}</p>
  {:else if searched && results.length === 0}
    <p class="text-copy text-subtle">{$locale && m.add_no_results()}</p>
  {:else}
    <ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {#each cards || [] as card (card.result.id)}
        <li
          class="flex gap-3 rounded-card border border-default bg-card p-3 {card.blacklisted
            ? 'opacity-50'
            : ''}"
        >
          <div class="h-[92px] w-[64px] shrink-0 overflow-hidden">
            <Cover src={card.result.cover} alt={card.result.title} radiusClass="rounded-field" />
          </div>
          <div class="flex min-w-0 flex-1 flex-col gap-1.5">
            <a
              href={card.anilistHref}
              target="_blank"
              rel="noopener noreferrer"
              title={card.result.title}
              aria-label="{card.result.title} — {$locale && m.add_link_anilist()}"
              class="flex items-center gap-1 hover:underline"
            >
              <span class="truncate text-copy text-heading">{card.result.title}</span>
              <ExternalLink size={12} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
            </a>
            <p class="truncate font-mono text-caption text-subtle">{card.meta}</p>
            {#if card.note}
              <p class="truncate text-caption text-subtle">{card.note}</p>
            {/if}
            <div class="mt-auto self-start">
              {#if card.statusHref}
                <Button variant="ghost" href={card.statusHref}>
                  {$locale && m.add_btn_view_status()}
                  <ArrowRight size={13} strokeWidth={2} aria-hidden="true" />
                </Button>
              {:else}
                <Button
                  variant="solid"
                  disabled={card.blacklisted || card.isAdding}
                  on:click={() => add(card.result)}
                >
                  {#if card.isAdding}
                    {$locale && m.add_btn_adding()}
                  {:else}
                    {$locale && m.add_btn_add()}
                  {/if}
                </Button>
              {/if}
            </div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
