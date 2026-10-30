<script lang="ts">
  // Torrent picker — one screen for Add (`#/add/:id/torrents`) and whole-title replace
  // (`#/status/:id/torrents`). The hash prefix is the back target; svelte-spa-router params
  // only carry `id`, so tests (and a reload) parse the hash instead of taking a mode prop.
  // Two confirms: paste does not override the list button (decisions.md #114).
  import { onDestroy, onMount } from "svelte";
  import { get } from "svelte/store";
  import { location, pop, push } from "svelte-spa-router";
  import { ExternalLink, Search } from "@lucide/svelte";
  import {
    ApiError,
    addStandaloneAnime,
    getTorrentCandidateDetail,
    getTorrentCandidates,
    replaceAnimeWithMagnet,
    type TorrentCandidate,
    type TorrentCandidateDetail,
    type TorrentCandidateMedia,
  } from "../lib/api/client.js";
  import { pickerCache } from "../lib/stores/pickerCache.js";
  import Button from "../components/ui/Button.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Cover from "../components/ui/Cover.svelte";
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
    queryLabel: m.picker_query_label(),
    search: m.picker_search(),
    hint: m.picker_hint(),
    confirm: m.picker_confirm(),
    empty: m.picker_empty(),
    nyaaError: m.picker_nyaa_error(),
    detailError: m.picker_detail_error(),
    openNyaa: m.picker_open_nyaa(),
    magnetPlaceholder: m.picker_magnet_placeholder(),
    magnetLabel: m.picker_magnet_label(),
    magnetConfirm: m.picker_magnet_confirm(),
    files: m.picker_files(),
  };

  let query = "";
  let items: TorrentCandidate[] = [];
  let listError: ApiError | null = null;
  let loading = false;
  let listSettled = false;
  let selectedIndex = 0;
  let detail: TorrentCandidateDetail | null = null;
  let detailError = false;
  let detailLoading = false;
  let pastedMagnet = "";
  let confirming = false;
  let media: TorrentCandidateMedia | null = null;

  let candidatesAbort: AbortController | undefined;
  let detailAbort: AbortController | undefined;

  $: selected = items[selectedIndex];
  $: canDownloadThis = Boolean(selected?.magnet) && !confirming && !loading;
  $: canDownloadMagnet = Boolean(pastedMagnet.trim()) && !confirming && !loading;
  $: nyaaHref = selected
    ? detail?.nyaa_url || `https://nyaa.si/view/${selected.nyaa_id}`
    : "";
  $: backLabel = !$locale
    ? ""
    : mode === "add"
      ? m.picker_back_add()
      : m.picker_back_replace({ title: media?.title || m.picker_title() });
  $: mediaMeta = !$locale || !media
    ? ""
    : [media.format.replace(/_/g, " "), media.episodes != null ? m.status_episodes_count({ count: media.episodes }) : ""]
        .filter(Boolean)
        .join(" · ");

  onMount(() => {
    const cached = get(pickerCache);
    if (cached && cached.mediaId === mediaId) {
      query = cached.query;
      items = cached.items;
      listError = cached.error;
      media = cached.media ?? null;
      listSettled = true;
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
      if (found.media) media = found.media;
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
      if (!controller.signal.aborted) {
        loading = false;
        listSettled = true;
      }
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

  async function afterSuccess(added?: { added: number }): Promise<void> {
    pickerCache.clear();
    if (mode === "replace") {
      toast.success(m.detail_replace_anime_done());
      await push(backHref.replace(/^#/, ""));
      return;
    }
    const count = added?.added ?? 0;
    if (count > 0) {
      toast.success(m.add_toast_added({ count }), undefined, {
        href: "#/downloads",
        label: m.add_toast_downloads_link(),
      });
    } else {
      toast.info(m.add_toast_tracked_no_episodes());
    }
    await pop();
  }

  async function confirmSelected(): Promise<void> {
    if (!selected?.magnet) return;
    confirming = true;
    try {
      if (mode === "replace") {
        await replaceAnimeWithMagnet(mediaId, selected.magnet);
        await afterSuccess();
        return;
      }
      const added = await addStandaloneAnime(mediaId, {
        magnet: selected.magnet,
        name: selected.name,
        nyaa_id: selected.nyaa_id,
        size_bytes: selected.size_bytes,
      });
      await afterSuccess(added);
    } catch (error) {
      toast.error(confirmErrorMessage(error));
    } finally {
      confirming = false;
    }
  }

  async function confirmMagnet(): Promise<void> {
    const pasted = pastedMagnet.trim();
    if (!pasted) return;
    confirming = true;
    try {
      if (mode === "replace") {
        await replaceAnimeWithMagnet(mediaId, pasted);
        await afterSuccess();
        return;
      }
      const added = await addStandaloneAnime(mediaId, { magnet: pasted });
      await afterSuccess(added);
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
  <div class="flex flex-col gap-3">
    <a href={backHref} class="w-fit text-copy text-body hover:text-heading">{backLabel}</a>
    <div class="flex items-start gap-3">
      <div class="h-[68px] w-[48px] shrink-0 overflow-hidden">
        <Cover src={media?.cover_image} alt={media?.title ?? ""} radiusClass="rounded-field" />
      </div>
      <div class="min-w-0">
        <p class="text-caption text-subtle">{T && T.title}</p>
        {#if media?.title}
          <h1 class="truncate text-screen-title text-heading">{media.title}</h1>
        {/if}
        {#if mediaMeta}
          <p class="mt-0.5 font-mono text-caption text-subtle">{mediaMeta}</p>
        {/if}
      </div>
    </div>
    <p class="text-copy text-subtle">{T && T.hint}</p>
  </div>

  <form class="flex flex-col gap-1.5" on:submit|preventDefault={onQuerySubmit}>
    <span class="text-caption text-subtle">{T && T.queryLabel}</span>
    <div class="flex items-center gap-2">
      <label
        class="flex flex-1 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-2 focus-within:border-accent"
      >
        <Search size={15} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
        <Chip variant="neutral">Nyaa</Chip>
        <input
          type="search"
          bind:value={query}
          placeholder={T && T.queryPlaceholder}
          aria-label={T && T.queryPlaceholder}
          class="w-full bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
        />
      </label>
      <Button variant="ghost" type="submit">{T && T.search}</Button>
    </div>
  </form>

  {#if listSettled && !loading}
    <p class="text-caption text-subtle">{$locale && m.picker_pack_count({ count: items.length })}</p>
  {/if}

  <div class="grid grid-cols-1 gap-4 md:grid-cols-[1.15fr_0.85fr] md:items-start">
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
      <div class="flex justify-end border-t border-default px-3.5 py-3">
        <Button variant="solid" disabled={!canDownloadThis} on:click={confirmSelected}>
          {T && T.confirm}
        </Button>
      </div>
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
          <div
            class="max-h-80 overflow-x-auto overflow-y-auto [overflow-wrap:anywhere] text-copy text-body [&_a]:text-accent [&_img]:max-w-full [&_pre]:overflow-x-auto [&_pre]:whitespace-pre-wrap [&_table]:border [&_table]:border-default [&_th]:border [&_th]:border-default [&_td]:border [&_td]:border-default [&_th]:px-2 [&_td]:px-2"
          >
            {@html detail.description_html}
          </div>
          {#if detail.files?.length}
            <h2 class="mt-4 text-card-title text-heading">{T && T.files}</h2>
            <ul class="mt-2 space-y-1 font-mono text-caption text-subtle">
              {#each detail.files ?? [] as file (file.name)}
                <li class="flex justify-between gap-2 hover:bg-control">
                  <span class="min-w-0 truncate hover:text-heading" title={file.name}>{file.name}</span>
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

  <div class="flex flex-col gap-1.5">
    <span class="text-caption text-subtle">{T && T.magnetLabel}</span>
    <div class="flex items-center gap-2">
      <input
        type="text"
        bind:value={pastedMagnet}
        placeholder={T && T.magnetPlaceholder}
        aria-label={T && T.magnetPlaceholder}
        autocomplete="off"
        spellcheck="false"
        class="w-full rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle focus:border-accent"
      />
      <Button variant="ghost" disabled={!canDownloadMagnet} on:click={confirmMagnet}>
        {T && T.magnetConfirm}
      </Button>
    </div>
  </div>
</div>
