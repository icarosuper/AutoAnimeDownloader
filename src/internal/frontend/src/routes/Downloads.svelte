<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { querystring, replace } from "svelte-spa-router";
  import {
    getTorrents,
    pauseTorrent,
    resumeTorrent,
    announceTorrent,
    deleteTorrent,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { formatSpeed, formatEta, formatPercent } from "../lib/utils/torrents.js";
  import { formatBytes } from "../lib/utils/status.js";
  import { statusLabel, statusClass } from "../lib/utils/torrentStatus.js";
  import {
    filterTorrents,
    sortTorrents,
    encodeViewState,
    decodeViewState,
    DEFAULT_VIEW_STATE,
    type ViewState,
    type SortKey,
  } from "../lib/utils/torrentFilters.js";
  import Loading from "../components/Loading.svelte";
  import DownloadsToolbar from "../components/DownloadsToolbar.svelte";
  import TorrentDeleteDialog from "../components/TorrentDeleteDialog.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.downloads_title(),
    subtitle: m.downloads_subtitle(),
    emptyTitle: m.downloads_empty_title(),
    emptyDesc: m.downloads_empty_desc(),
    emptyFilteredTitle: m.downloads_empty_filtered_title(),
    emptyFilteredDesc: m.downloads_empty_filtered_desc(),
    clearFilters: m.downloads_clear_filters(),
    colName: m.downloads_col_name(),
    colProgress: m.downloads_col_progress(),
    colSpeed: m.downloads_col_speed(),
    colEta: m.downloads_col_eta(),
    colPeers: m.downloads_col_peers(),
    colActions: m.downloads_col_actions(),
    pause: m.downloads_pause(),
    resume: m.downloads_resume(),
    announce: m.downloads_announce(),
    delete: m.downloads_delete(),
    batch: m.downloads_batch(),
    selectAll: m.downloads_select_all(),
  };

  let torrents: TorrentInfo[] = [];
  let loading = true;
  // Hashes com ação em voo: desabilitam os botões daquela linha sem congelar a tabela toda.
  let busy = new Set<string>();

  // Estado da view (busca/filtro/ordenação) vive na URL, não em localStorage — veja
  // encodeViewState/decodeViewState em torrentFilters.ts.
  let viewState: ViewState = DEFAULT_VIEW_STATE;

  // Seleção em massa: Set de hashes, sempre podado para o que está visível (pós-filtro).
  let selected = new Set<string>();
  let bulkBusy = false;

  // Diálogo de exclusão — serve tanto para 1 linha quanto para o lote selecionado.
  let deleteDialogOpen = false;
  let pendingDeleteHashes: string[] = [];
  let pendingDeleteName = "";

  function canPause(t: TorrentInfo): boolean {
    return t.status !== "stopped" && t.status !== "stopping";
  }
  function canResume(t: TorrentInfo): boolean {
    return t.status === "stopped";
  }

  // Ordena só quando o usuário clica num cabeçalho; 'default' preserva a ordem do backend.
  $: visibleTorrents = sortTorrents(filterTorrents(torrents, viewState), viewState.sort, viewState.dir);

  // Poda a seleção sempre que a lista visível mudar (filtro/busca mudou, ou o polling trouxe/tirou
  // um torrent). Não referencia `selected` diretamente na expressão do $: para não criar um loop
  // reativo — a leitura/escrita acontece dentro da função.
  $: pruneSelectionToVisible(visibleTorrents);

  function pruneSelectionToVisible(visible: TorrentInfo[]) {
    const visibleHashes = new Set(visible.map((t) => t.hash));
    let changed = false;
    const next = new Set<string>();
    selected.forEach((h) => {
      if (visibleHashes.has(h)) next.add(h);
      else changed = true;
    });
    if (changed) selected = next;
  }

  $: allVisibleSelected = visibleTorrents.length > 0 && visibleTorrents.every((t) => selected.has(t.hash));
  $: someVisibleSelected = selected.size > 0 && !allVisibleSelected;

  function toggleSelectAll() {
    // Select-all marca só as linhas visíveis (pós-filtro), nunca a lista inteira.
    selected = allVisibleSelected ? new Set() : new Set(visibleTorrents.map((t) => t.hash));
  }

  function toggleSelect(hash: string) {
    const next = new Set(selected);
    if (next.has(hash)) next.delete(hash);
    else next.add(hash);
    selected = next;
  }

  function syncUrl() {
    const qs = encodeViewState(viewState);
    replace(`/downloads${qs ? `?${qs}` : ""}`);
  }

  function setQuery(q: string) {
    viewState = { ...viewState, query: q };
    syncUrl();
  }

  function setStatuses(statuses: string[]) {
    viewState = { ...viewState, statuses };
    syncUrl();
  }

  function clearFilters() {
    viewState = { ...viewState, query: "", statuses: [] };
    syncUrl();
  }

  function setSort(key: SortKey) {
    if (viewState.sort === key) {
      viewState = { ...viewState, dir: viewState.dir === "asc" ? "desc" : "asc" };
    } else {
      viewState = { ...viewState, sort: key, dir: "asc" };
    }
    syncUrl();
  }

  function sortIndicator(key: SortKey): string {
    if (viewState.sort !== key) return "";
    return viewState.dir === "asc" ? "▲" : "▼";
  }

  async function load() {
    try {
      torrents = await getTorrents();
    } catch (err) {
      console.error("Failed to load torrents:", err);
    } finally {
      loading = false;
    }
  }

  async function runAction(hash: string, action: (h: string) => Promise<void>) {
    busy = new Set(busy).add(hash);
    try {
      await action(hash);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      const next = new Set(busy);
      next.delete(hash);
      busy = next;
    }
  }

  // Ações em lote: cada uma filtra o que faz sentido (ex.: não manda pausar quem já está
  // stopped/stopping) e dispara N requisições aos endpoints por hash já existentes — não há
  // endpoint de lote no backend.
  async function runBulk(
    hashes: string[],
    action: (h: string) => Promise<void>,
  ): Promise<{ success: number; failed: number; total: number }> {
    bulkBusy = true;
    let success = 0;
    let failed = 0;
    await Promise.all(
      hashes.map(async (h) => {
        try {
          await action(h);
          success++;
        } catch {
          failed++;
        }
      }),
    );
    bulkBusy = false;
    return { success, failed, total: hashes.length };
  }

  function reportBulk(
    result: { success: number; failed: number; total: number },
    doneMsg: (a: { success: number; total: number }) => string,
  ) {
    if (result.total === 0) {
      toast.info(m.downloads_bulk_none());
      return;
    }
    if (result.success > 0) toast.success(doneMsg({ success: result.success, total: result.total }));
    if (result.failed > 0) toast.error(m.downloads_bulk_partial_error({ failed: result.failed }));
  }

  async function handleBulkPause() {
    const targets = visibleTorrents.filter((t) => selected.has(t.hash) && canPause(t)).map((t) => t.hash);
    const result = await runBulk(targets, pauseTorrent);
    reportBulk(result, m.downloads_bulk_paused);
    selected = new Set();
    await load();
  }

  async function handleBulkResume() {
    const targets = visibleTorrents.filter((t) => selected.has(t.hash) && canResume(t)).map((t) => t.hash);
    const result = await runBulk(targets, resumeTorrent);
    reportBulk(result, m.downloads_bulk_resumed);
    selected = new Set();
    await load();
  }

  async function handleBulkAnnounce() {
    const targets = [...selected];
    const result = await runBulk(targets, announceTorrent);
    reportBulk(result, m.downloads_bulk_announced);
    selected = new Set();
    await load();
  }

  function handleDeleteRow(t: TorrentInfo) {
    pendingDeleteHashes = [t.hash];
    pendingDeleteName = t.anime_name || t.name;
    deleteDialogOpen = true;
  }

  function handleBulkDeleteRequest() {
    pendingDeleteHashes = [...selected];
    pendingDeleteName = "";
    deleteDialogOpen = true;
  }

  async function confirmDelete(e: CustomEvent<{ keepData: boolean; block: boolean }>) {
    const { keepData, block } = e.detail;
    const hashes = pendingDeleteHashes;
    pendingDeleteHashes = [];

    if (hashes.length <= 1) {
      const hash = hashes[0];
      if (!hash) return;
      busy = new Set(busy).add(hash);
      try {
        await deleteTorrent(hash, { keepData, block });
        toast.success(m.downloads_delete_toast_done());
      } catch (err) {
        toast.error(err instanceof Error ? err.message : m.downloads_delete_toast_error());
      } finally {
        const next = new Set(busy);
        next.delete(hash);
        busy = next;
      }
    } else {
      const result = await runBulk(hashes, (h) => deleteTorrent(h, { keepData, block }));
      reportBulk(result, m.downloads_bulk_deleted);
    }

    selected = new Set();
    await load();
  }

  function cancelDelete() {
    pendingDeleteHashes = [];
  }

  let pollInterval: ReturnType<typeof setInterval> | null = null;

  onMount(() => {
    viewState = decodeViewState($querystring ?? "");
    load();
    // Polling só enquanto a tela está montada: cada snapshot custa um Stats() por torrent,
    // que é round-trip bloqueante para dentro da goroutine de cada um.
    pollInterval = setInterval(load, 2000);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });
</script>

<TorrentDeleteDialog
  bind:open={deleteDialogOpen}
  count={pendingDeleteHashes.length || 1}
  name={pendingDeleteName}
  on:confirm={confirmDelete}
  on:cancel={cancelDelete}
/>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-base-content">{T && T.title}</h1>
    <p class="text-sm text-base-content/50 mt-0.5">{T && T.subtitle}</p>
  </div>

  {#if loading}
    <Loading message="Loading torrents..." />
  {:else if torrents.length === 0}
    <div class="card bg-base-200 border border-base-300">
      <div class="card-body items-center text-center py-12">
        <h2 class="text-lg font-medium text-base-content">{T && T.emptyTitle}</h2>
        <p class="text-sm text-base-content/50">{T && T.emptyDesc}</p>
      </div>
    </div>
  {:else}
    <DownloadsToolbar
      query={viewState.query}
      statuses={viewState.statuses}
      selectedCount={selected.size}
      {bulkBusy}
      on:search={(e) => setQuery(e.detail)}
      on:statusesChange={(e) => setStatuses(e.detail)}
      on:bulkPause={handleBulkPause}
      on:bulkResume={handleBulkResume}
      on:bulkAnnounce={handleBulkAnnounce}
      on:bulkDelete={handleBulkDeleteRequest}
      on:deselectAll={() => { selected = new Set(); }}
    />

    {#if visibleTorrents.length === 0}
      <div class="card bg-base-200 border border-base-300">
        <div class="card-body items-center text-center py-12">
          <h2 class="text-lg font-medium text-base-content">{T && T.emptyFilteredTitle}</h2>
          <p class="text-sm text-base-content/50">{T && T.emptyFilteredDesc}</p>
          <button class="btn btn-sm btn-ghost mt-2" on:click={clearFilters}>{T && T.clearFilters}</button>
        </div>
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="table table-sm">
          <thead>
            <tr>
              <th class="w-8">
                <input
                  type="checkbox"
                  class="checkbox checkbox-sm"
                  aria-label={T && T.selectAll}
                  checked={allVisibleSelected}
                  indeterminate={someVisibleSelected}
                  on:change={toggleSelectAll}
                />
              </th>
              <th class="cursor-pointer select-none" on:click={() => setSort("name")}>{T && T.colName} {sortIndicator("name")}</th>
              <th class="cursor-pointer select-none" on:click={() => setSort("progress")}>{T && T.colProgress} {sortIndicator("progress")}</th>
              <th class="cursor-pointer select-none" on:click={() => setSort("download_speed")}>{T && T.colSpeed} {sortIndicator("download_speed")}</th>
              <th class="cursor-pointer select-none" on:click={() => setSort("eta")}>{T && T.colEta} {sortIndicator("eta")}</th>
              <th class="cursor-pointer select-none" on:click={() => setSort("peers")}>{T && T.colPeers} {sortIndicator("peers")}</th>
              <th class="text-right">{T && T.colActions}</th>
            </tr>
          </thead>
          <tbody>
            {#each visibleTorrents as t (t.hash)}
              <tr class:bg-base-200={selected.has(t.hash)}>
                <td>
                  <input
                    type="checkbox"
                    class="checkbox checkbox-sm"
                    aria-label={T && m.downloads_select_row({ name: t.anime_name || t.name })}
                    checked={selected.has(t.hash)}
                    on:change={() => toggleSelect(t.hash)}
                  />
                </td>
                <td class="max-w-xs">
                  <div class="font-medium text-base-content truncate" title={t.name}>
                    {t.anime_name || t.name}
                  </div>
                  <div class="flex items-center gap-2 mt-0.5">
                    <span class="badge badge-xs {statusClass(t.status)}">{$locale && statusLabel(t.status)}</span>
                    {#if t.is_batch}
                      <span class="text-xs text-base-content/40">{T && T.batch}</span>
                    {:else if t.episode_number !== null}
                      <span class="text-xs text-base-content/40">
                        {$locale && m.downloads_episode({ number: t.episode_number })}
                      </span>
                    {/if}
                  </div>
                </td>
                <td class="min-w-[10rem]">
                  <!-- bytes_total fica em 0 até a metadata chegar; a barra some nesse intervalo -->
                  {#if t.bytes_total > 0}
                    <progress class="progress progress-primary w-full" value={t.progress} max="1"></progress>
                    <div class="text-xs text-base-content/40 mt-0.5">
                      <!-- Pausing zeroes bytes_completed (rain frees the piece data on Stop),
                           while progress keeps reporting the real fraction via the piece-count
                           fallback. Showing "0 B / 1.4 GB" next to a non-zero bar would read as
                           a contradiction, so drop the byte pair in that case. -->
                      {#if t.bytes_completed === 0 && t.progress > 0}
                        {formatPercent(t.progress)}
                      {:else}
                        {formatPercent(t.progress)} · {formatBytes(t.bytes_completed)} / {formatBytes(t.bytes_total)}
                      {/if}
                    </div>
                  {:else}
                    <progress class="progress w-full"></progress>
                    <div class="text-xs text-base-content/40 mt-0.5">—</div>
                  {/if}
                  <div class="text-xs text-base-content/30">
                    {$locale && m.downloads_uploaded({ size: formatBytes(t.bytes_uploaded) })}
                  </div>
                </td>
                <td class="whitespace-nowrap text-sm">
                  <div>↓ {formatSpeed(t.download_speed)}</div>
                  <div class="text-base-content/40">↑ {formatSpeed(t.upload_speed)}</div>
                </td>
                <td class="whitespace-nowrap text-sm">{formatEta(t.eta_seconds)}</td>
                <td class="text-sm">{t.peers_total}</td>
                <td>
                  <div class="flex justify-end gap-1">
                    {#if canResume(t)}
                      <button
                        class="btn btn-xs"
                        disabled={busy.has(t.hash)}
                        on:click={() => runAction(t.hash, resumeTorrent)}
                      >
                        {T && T.resume}
                      </button>
                    {:else}
                      <button
                        class="btn btn-xs"
                        disabled={busy.has(t.hash) || !canPause(t)}
                        on:click={() => runAction(t.hash, pauseTorrent)}
                      >
                        {T && T.pause}
                      </button>
                    {/if}
                    <button
                      class="btn btn-xs btn-ghost"
                      disabled={busy.has(t.hash)}
                      on:click={() => runAction(t.hash, announceTorrent)}
                    >
                      {T && T.announce}
                    </button>
                    <button
                      class="btn btn-xs btn-ghost text-error"
                      disabled={busy.has(t.hash)}
                      on:click={() => handleDeleteRow(t)}
                    >
                      {T && T.delete}
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>
