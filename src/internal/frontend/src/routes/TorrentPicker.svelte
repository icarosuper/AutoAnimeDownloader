<script lang="ts">
  // Torrent picker — one screen for Add (`#/add/:id/torrents`) and whole-title replace
  // (`#/status/:id/torrents`). The hash prefix is the back target; svelte-spa-router params
  // only carry `id`, so tests (and a reload) parse the hash instead of taking a mode prop.
  import { onDestroy, onMount } from "svelte";
  import { get } from "svelte/store";
  import { location, pop, push } from "svelte-spa-router";
  import { ChevronRight, ExternalLink, Search } from "@lucide/svelte";
  import {
    ApiError,
    addStandaloneAnime,
    getTorrentCandidateDetail,
    getTorrentCandidates,
    replaceAnimeWithMagnet,
    type TorrentCandidate,
    type TorrentCandidateDetail,
  } from "../lib/api/client.js";
  import { pickerCache } from "../lib/stores/pickerCache.js";
  import Button from "../components/ui/Button.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Loading from "../components/Loading.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { formatBytes, type FormatLocale } from "../lib/domain/format.js";

  export let params: { id?: string } = {};

  function parsePickerRoute(path: string): {
    id: number;
    mode: "add" | "replace";
    backHref: string;
  } {
    const cleaned = (path || "").replace(/^#/, "");
    const addMatch = cleaned.match(/^\/add\/(\d+)\/torrents\/?$/);
    if (addMatch) {
      const id = Number(addMatch[1]);
      return { id, mode: "add", backHref: "#/add" };
    }
    const statusMatch = cleaned.match(/^\/status\/(\d+)\/torrents\/?$/);
    if (statusMatch) {
      const id = Number(statusMatch[1]);
      return { id, mode: "replace", backHref: `#/status/${id}` };
    }
    return { id: 0, mode: "add", backHref: "#/add" };
  }

  $: parsed = parsePickerRoute($location || (typeof window !== "undefined" ? window.location.hash : ""));
  $: mediaId = parsed.id || parseInt(params.id || "0", 10);
  $: mode = parsed.mode;
  $: backHref = parsed.backHref;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;
  $: T = $locale && {
    title: m.picker_title(),
    recommended: m.picker_recommended(),
    queryPlaceholder: m.picker_query_placeholder(),
    confirm: m.picker_confirm(),
    back: m.picker_back(),
    empty: m.picker_empty(),
    nyaaError: m.picker_nyaa_error(),
    detailError: m.picker_detail_error(),
    openNyaa: m.picker_open_nyaa(),
    magnetPlaceholder: m.picker_magnet_placeholder(),
    files: m.picker_files(),
    crumb: mode === "add" ? m.add_title() : m.nav_status(),
  };

  let query = "";
  let items: TorrentCandidate[] = [];
  let listError: ApiError | null = null;
  let loading = false;
  let selectedIndex = 0;
  let detail: TorrentCandidateDetail | null = null;
  let detailError = false;
  let detailLoading = false;
  let pastedMagnet = "";
  let confirming = false;

  let candidatesAbort: AbortController | undefined;
  let detailAbort: AbortController | undefined;

  $: selected = items[selectedIndex];
  $: canConfirm = Boolean(pastedMagnet.trim() || selected?.magnet) && !confirming && !loading;
  $: nyaaHref = selected
    ? detail?.nyaa_url || `https://nyaa.si/view/${selected.nyaa_id}`
    : "";

  onMount(() => {
    const cached = get(pickerCache);
    if (cached && cached.mediaId === mediaId) {
      query = cached.query;
      items = cached.items;
      listError = cached.error;
      if (items.length > 0) {
        selectedIndex = 0;
        loadDetail(items[0].nyaa_id);
      }
      return;
    }
    loadCandidates();
  });

  onDestroy(() => {
    candidatesAbort?.abort();
    detailAbort?.abort();
  });

  function onQuerySubmit(): void {
    const q = query.trim();
    if (!q) return;
    loadCandidates(q);
  }

  async function loadCandidates(q?: string): Promise<void> {
    candidatesAbort?.abort();
    detailAbort?.abort();
    const controller = new AbortController();
    candidatesAbort = controller;
    loading = true;
    listError = null;
    detail = null;
    detailError = false;

    try {
      const found = await getTorrentCandidates(mediaId, q, controller.signal);
      if (controller.signal.aborted) return;
      items = found.items ?? [];
      query = found.query ?? q ?? "";
      selectedIndex = 0;
      if (items.length > 0) loadDetail(items[0].nyaa_id);
    } catch (error) {
      if (controller.signal.aborted || (error instanceof Error && error.name === "AbortError")) return;
      items = [];
      detail = null;
      listError =
        error instanceof ApiError
          ? error
          : new ApiError(error instanceof Error ? error.message : "error", "NYAA_UNAVAILABLE", 0);
    } finally {
      if (!controller.signal.aborted) loading = false;
    }
  }

  function select(index: number): void {
    if (index === selectedIndex && detail) return;
    selectedIndex = index;
    const item = items[index];
    if (item) loadDetail(item.nyaa_id);
  }

  async function loadDetail(nyaaId: number): Promise<void> {
    detailAbort?.abort();
    const controller = new AbortController();
    detailAbort = controller;
    detailLoading = true;
    detailError = false;
    detail = null;
    try {
      const next = await getTorrentCandidateDetail(mediaId, nyaaId, controller.signal);
      if (controller.signal.aborted) return;
      detail = next;
    } catch (error) {
      if (controller.signal.aborted || (error instanceof Error && error.name === "AbortError")) return;
      detailError = true;
      detail = null;
    } finally {
      if (!controller.signal.aborted) detailLoading = false;
    }
  }

  async function confirm(): Promise<void> {
    const pasted = pastedMagnet.trim();
    const magnet = pasted || selected?.magnet;
    if (!magnet) return;
    confirming = true;
    try {
      if (mode === "replace") {
        await replaceAnimeWithMagnet(mediaId, magnet);
        toast.success(m.detail_replace_anime_done());
        pickerCache.clear();
        await push(backHref.replace(/^#/, ""));
        return;
      }
      const opts = pasted
        ? { magnet: pasted }
        : selected
          ? {
              magnet: selected.magnet,
              name: selected.name,
              nyaa_id: selected.nyaa_id,
              size_bytes: selected.size_bytes,
            }
          : { magnet };
      const { added: count } = await addStandaloneAnime(mediaId, opts);
      pickerCache.clear();
      if (count > 0) {
        toast.success(m.add_toast_added({ count }), undefined, {
          href: "#/downloads",
          label: m.add_toast_downloads_link(),
        });
      } else {
        toast.info(m.add_toast_tracked_no_episodes());
      }
      await pop();
    } catch (error) {
      toast.error(confirmErrorMessage(error));
    } finally {
      confirming = false;
    }
  }

  function confirmErrorMessage(error: unknown): string {
    if (mode === "replace") {
      return error instanceof Error ? error.message : m.detail_replace_anime_error();
    }
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

  function metaLine(item: TorrentCandidate): string {
    const parts: string[] = [];
    if (item.fansub) parts.push(item.fansub);
    if (item.resolution) parts.push(item.resolution);
    if (item.is_batch && item.range_end) parts.push(`${item.range_start}–${item.range_end}`);
    return parts.join(" · ");
  }
</script>

<div class="flex flex-col gap-4.5">
  <div>
    <nav class="flex items-center gap-1 text-caption text-subtle" aria-label={T && T.back}>
      <a href={backHref} class="hover:text-body">{T && T.crumb}</a>
      <ChevronRight size={13} strokeWidth={2} aria-hidden="true" />
      <span class="truncate text-body">{T && T.title}</span>
    </nav>
    <h1 class="mt-2 text-screen-title text-heading">{T && T.title}</h1>
  </div>

  <form class="flex items-center gap-2" on:submit|preventDefault={onQuerySubmit}>
    <label
      class="flex flex-1 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-2 focus-within:border-accent"
    >
      <Search size={15} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
      <input
        type="search"
        bind:value={query}
        placeholder={T && T.queryPlaceholder}
        aria-label={T && T.queryPlaceholder}
        class="w-full bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
      />
    </label>
  </form>

  <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
    <div class="rounded-card border border-default bg-card">
      {#if loading}
        <Loading message={T && T.queryPlaceholder} />
      {:else if listError}
        <p role="alert" class="p-4 text-copy text-danger">{T && T.nyaaError}</p>
      {:else if items.length === 0}
        <p class="p-4 text-copy text-subtle">{T && T.empty}</p>
      {:else}
        <ul role="listbox" aria-label={T && T.title} class="divide-y divide-divider">
          {#each items as item, i (item.nyaa_id)}
            <li
              role="option"
              aria-selected={i === selectedIndex}
              tabindex="0"
              class="cursor-pointer px-3.5 py-3 {i === selectedIndex
                ? 'bg-accent-tint/12'
                : 'hover:bg-control'}"
              on:click={() => select(i)}
              on:keydown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  select(i);
                }
              }}
            >
              <div class="flex items-start justify-between gap-2">
                <p class="min-w-0 truncate text-copy text-heading" title={item.name}>{item.name}</p>
                {#if i === 0}
                  <Chip variant="accent">{T && T.recommended}</Chip>
                {/if}
              </div>
              <p class="mt-1 font-mono text-caption text-subtle">
                {$locale && m.picker_seeders({ count: item.seeders })}
                · {formatBytes(item.size_bytes, fmtLocale)}
                {#if metaLine(item)}
                  · {metaLine(item)}
                {/if}
              </p>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="rounded-card border border-default bg-card p-4">
      {#if !selected}
        <p class="text-copy text-subtle">{T && T.empty}</p>
      {:else}
        {#if detailLoading}
          <Loading message={T && T.openNyaa} />
        {:else if detailError}
          <p role="alert" class="text-copy text-danger">{T && T.detailError}</p>
        {:else if detail}
          <div class="text-copy text-body [&_a]:text-accent [&_img]:max-w-full">
            {@html detail.description_html}
          </div>
          {#if detail.files.length > 0}
            <h2 class="mt-4 text-card-title text-heading">{T && T.files}</h2>
            <ul class="mt-2 space-y-1 font-mono text-caption text-subtle">
              {#each detail.files as file (file.name)}
                <li class="flex justify-between gap-2">
                  <span class="min-w-0 truncate" title={file.name}>{file.name}</span>
                  <span class="shrink-0">{formatBytes(file.size_bytes, fmtLocale)}</span>
                </li>
              {/each}
            </ul>
          {/if}
        {/if}
        {#if nyaaHref}
          <a
            href={nyaaHref}
            target="_blank"
            rel="noopener noreferrer"
            class="mt-4 inline-flex items-center gap-1 text-copy text-accent hover:underline"
          >
            {T && T.openNyaa}
            <ExternalLink size={12} strokeWidth={2} aria-hidden="true" />
          </a>
        {/if}
      {/if}
    </div>
  </div>

  <input
    type="text"
    bind:value={pastedMagnet}
    placeholder={T && T.magnetPlaceholder}
    aria-label={T && T.magnetPlaceholder}
    autocomplete="off"
    spellcheck="false"
    class="w-full rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle focus:border-accent"
  />

  <div
    class="sticky bottom-0 z-10 -mx-4 flex items-center justify-between gap-2 border-t border-default bg-window px-4 py-3 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8 max-md:bottom-16"
  >
    <Button variant="ghost" href={backHref}>{T && T.back}</Button>
    <Button variant="solid" disabled={!canConfirm} on:click={confirm}>
      {T && T.confirm}
    </Button>
  </div>
</div>
