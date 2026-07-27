<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    getTorrents,
    pauseTorrent,
    resumeTorrent,
    announceTorrent,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { formatSpeed, formatEta, formatPercent } from "../lib/utils/torrents.js";
  import { formatBytes } from "../lib/utils/status.js";
  import Loading from "../components/Loading.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.downloads_title(),
    subtitle: m.downloads_subtitle(),
    emptyTitle: m.downloads_empty_title(),
    emptyDesc: m.downloads_empty_desc(),
    colName: m.downloads_col_name(),
    colProgress: m.downloads_col_progress(),
    colSpeed: m.downloads_col_speed(),
    colEta: m.downloads_col_eta(),
    colPeers: m.downloads_col_peers(),
    colActions: m.downloads_col_actions(),
    pause: m.downloads_pause(),
    resume: m.downloads_resume(),
    announce: m.downloads_announce(),
    batch: m.downloads_batch(),
  };

  let torrents: TorrentInfo[] = [];
  let loading = true;
  // Hashes com ação em voo: desabilitam os botões daquela linha sem congelar a tabela toda.
  let busy = new Set<string>();

  // O slug de status vem do backend; o mapa é exaustivo com statusSlug() do Go.
  function statusLabel(status: string): string {
    switch (status) {
      case "stopped": return m.downloads_status_stopped();
      case "downloading_metadata": return m.downloads_status_downloading_metadata();
      case "allocating": return m.downloads_status_allocating();
      case "verifying": return m.downloads_status_verifying();
      case "downloading": return m.downloads_status_downloading();
      case "seeding": return m.downloads_status_seeding();
      case "stopping": return m.downloads_status_stopping();
      default: return m.downloads_status_unknown();
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case "seeding": return "badge-success";
      case "downloading": return "badge-info";
      case "stopped": return "badge-ghost";
      case "stopping": return "badge-warning";
      default: return "badge-neutral";
    }
  }

  // "stopping" dura até ~5s depois do Stop(); durante ele nem pausar nem retomar faz sentido.
  function canPause(t: TorrentInfo): boolean {
    return t.status !== "stopped" && t.status !== "stopping";
  }
  function canResume(t: TorrentInfo): boolean {
    return t.status === "stopped";
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

  let pollInterval: ReturnType<typeof setInterval> | null = null;

  onMount(() => {
    load();
    // Polling só enquanto a tela está montada: cada snapshot custa um Stats() por torrent,
    // que é round-trip bloqueante para dentro da goroutine de cada um.
    pollInterval = setInterval(load, 2000);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });
</script>

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
    <div class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{T && T.colName}</th>
            <th>{T && T.colProgress}</th>
            <th>{T && T.colSpeed}</th>
            <th>{T && T.colEta}</th>
            <th>{T && T.colPeers}</th>
            <th class="text-right">{T && T.colActions}</th>
          </tr>
        </thead>
        <tbody>
          {#each torrents as t (t.hash)}
            <tr>
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
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
