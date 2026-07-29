<script lang="ts">
  // Status — spec §9.1 (Fase 3). Continua sendo daemon E lista de animes (decisão D4): a tela
  // "Biblioteca" separada do handoff não existe. Layout novo de cima para baixo: header com a
  // pílula do daemon, card herói de velocidade, coluna direita (biblioteca + disco/próxima
  // verificação) e a lista de animes.
  //
  // A seção "Fila e problemas" do artboard 1b foi DESCARTADA de propósito (spec §9.1): é
  // redundante com a tela de Downloads, que existe justamente para isso.
  import { onMount, onDestroy } from "svelte";
  import { ChevronRight, Eye, Search, X } from "@lucide/svelte";
  import {
    getStatus,
    getAnimes,
    getConfig,
    triggerCheck,
    startDaemon,
    stopDaemon,
    getTorrents,
    type StatusResponse,
    type AnimeInfo,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import Loading from "../components/Loading.svelte";
  import Button from "../components/ui/Button.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import ProgressRing from "../components/ui/ProgressRing.svelte";
  import PulseDot from "../components/ui/PulseDot.svelte";
  import Sparkline from "../components/ui/Sparkline.svelte";
  import TripleProgressBar from "../components/ui/TripleProgressBar.svelte";
  import { toast } from "../lib/stores/toast.js";
  import { wsConnectionState } from "../lib/stores/wsState.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { filterAnimes, sortAnimes, computeNextCheckIn, isDiskSpaceLow, type SortKey, type SortDir } from "../lib/utils/status.js";
  import { totalSpeeds } from "../lib/utils/torrents.js";
  import { deriveAnimeChip, type AnimeChipState } from "../lib/domain/animeState.js";
  import {
    formatBytes,
    formatDate as formatDateDomain,
    formatEta,
    formatSpeed,
    formatSpeedParts,
    type FormatLocale,
  } from "../lib/domain/format.js";
  import { speedHistory } from "../lib/stores/speedHistory.js";
  import { stallTracker } from "../lib/stores/stallTracker.js";

  // Intervalo do poll de torrents. Exposto como constante porque a nota "polling Ns" mostrada
  // ao usuário (spec §8: a UI declara que progresso vem de polling HTTP, não do WebSocket) é
  // derivada daqui — nunca um número escrito à mão no markup que possa divergir do timer.
  const TORRENTS_POLL_MS = 5000;

  // Reactive translations — re-evaluated when $locale changes, no remount needed
  $: T = $locale && {
    title: m.status_title(),
    subtitle: m.status_subtitle(),
    cardLibrary: m.status_card_library(),
    cardDisk: m.status_card_disk(),
    cardNextCheck: m.status_card_next_check(),
    checking: m.status_checking(),
    never: m.common_never(),
    errorAlert: m.status_error_alert(),
    start: m.status_start(),
    starting: m.status_starting(),
    stop: m.status_stop(),
    stopping: m.status_stopping(),
    forceCheck: m.status_force_check(),
    animesHeader: m.status_animes_header(),
    searchPlaceholder: m.status_search_placeholder(),
    clearSearch: m.status_clear_search(),
    colName: m.status_col_name(),
    colState: m.status_col_state(),
    colProgress: m.status_col_progress(),
    colLastDownload: m.status_col_last_download(),
    emptyTitle: m.status_empty_title(),
    emptyDesc: m.status_empty_desc(),
    goToConfig: m.status_go_to_config(),
    filterUnwatched: m.status_filter_unwatched(),
    heroSpeedLabel: m.status_hero_speed_label(),
    heroNoDownloads: m.status_hero_no_downloads(),
    activeDownloads: m.status_active_downloads_label(),
    pollingNote: m.status_polling_note({ seconds: TORRENTS_POLL_MS / 1000 }),
    staleNote: m.status_stale_note(),
    sortAsc: m.status_sort_asc(),
    sortDesc: m.status_sort_desc(),
  };

  let status: StatusResponse | null = null;
  let animes: AnimeInfo[] = [];
  let torrents: TorrentInfo[] = [];
  let checkInterval = 0;
  let loading = true;
  let actionLoading = false;
  let search = "";
  let filterUnwatched = false;
  let now = Date.now();
  // spec §8: o progresso não vem do WebSocket, vem do poll HTTP. Quando um poll falha a UI
  // precisa DIZER isso em vez de continuar exibindo o último número como se fosse ao vivo.
  let torrentsStale = false;

  let sortKey: SortKey = "last_download_date";
  let sortDir: SortDir = "desc";

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  $: filteredAnimes = filterAnimes(animes, search, filterUnwatched);
  $: sortedAnimes = sortAnimes(filteredAnimes, sortKey, sortDir);

  $: nextCheckIn = status
    ? computeNextCheckIn(status.last_check, checkInterval, status.status, now)
    : null;

  $: diskSpaceLow = status ? isDiskSpaceLow(status.disk_free, status.disk_total) : false;

  $: speeds = totalSpeeds(torrents);
  $: heroSpeed = formatSpeedParts(speeds.download, fmtLocale);

  $: activeDownloads = torrents.filter((t) => t.status === "downloading");
  $: seedingCount = torrents.filter((t) => t.status === "seeding").length;

  /**
   * Normaliza os quatro contadores de um anime para o que a TripleProgressBar espera:
   * watched <= downloaded <= released <= total, todos dentro do mesmo denominador.
   *
   * O clamp em cascata não é cosmético. `total_episodes` vem da AniList e é 0 enquanto o anime
   * está em exibição sem contagem anunciada — nesse caso o denominador passa a ser
   * `episodes_released` (mesma regra da tela anterior). E os contadores podem se cruzar na
   * prática: o daemon marca um episódio como assistido a partir do progresso da AniList, que o
   * usuário atualiza sozinho, então `episodes_watched` pode passar `episodes_downloaded` sem
   * que nada esteja errado. Sem o `Math.max` encadeado a barra renderizaria um segmento de
   * largura negativa.
   */
  function breakdown(a: AnimeInfo) {
    const total = a.total_episodes > 0 ? a.total_episodes : a.episodes_released;
    const watched = Math.min(Math.max(a.episodes_watched, 0), total);
    const downloaded = Math.min(Math.max(a.episodes_downloaded, watched), total);
    const released = Math.min(Math.max(a.episodes_released, downloaded), total);
    return { watched, downloaded, released, total };
  }

  function legendFor(b: { watched: number; downloaded: number; released: number; total: number }) {
    return m.status_library_legend({
      watched: b.watched,
      downloaded: b.downloaded,
      released: b.released,
      total: b.total,
    });
  }

  // Resumo da biblioteca: soma dos breakdowns já normalizados (não dos campos crus), para que
  // o agregado obedeça à mesma invariante watched <= downloaded <= released <= total.
  $: libraryTotals = animes.reduce(
    (acc, a) => {
      const b = breakdown(a);
      return {
        watched: acc.watched + b.watched,
        downloaded: acc.downloaded + b.downloaded,
        released: acc.released + b.released,
        total: acc.total + b.total,
      };
    },
    { watched: 0, downloaded: 0, released: 0, total: 0 },
  );

  function chipLabel(state: AnimeChipState): string {
    // `deriveAnimeChip` devolve chave + números crus, nunca texto (ver o comentário de
    // i18n em lib/domain/animeState.ts). A tradução acontece aqui.
    switch (state.key) {
      case "blacklisted":
        return m.chip_blacklisted();
      case "downloading":
        return state.episodeNumber === undefined
          ? m.chip_downloading_no_episode({ percent: state.percent ?? 0 })
          : m.chip_downloading({ episode: state.episodeNumber, percent: state.percent ?? 0 });
      case "noSeeds":
        return m.chip_no_seeds();
      case "paused":
        return m.chip_paused();
      case "awaitingPremiere":
        return m.chip_awaiting_premiere();
      case "upToDate":
        return m.chip_up_to_date();
      case "behind":
        return m.chip_behind({ count: state.behindCount ?? 0 });
    }
  }

  // Uma passada só monta tudo que cada linha precisa. Depende de $locale (textos), `torrents`
  // e $stallTracker (chip) e `now` (limiar de "sem seeds" e tempo relativo).
  $: rows =
    $locale && sortedAnimes.map((anime) => {
      const chip = deriveAnimeChip(anime, torrents, now, $stallTracker);
      const b = breakdown(anime);
      return {
        anime,
        chip,
        chipText: chipLabel(chip),
        breakdown: b,
        legend: legendFor(b),
        href: anime.anime_id ? `#/status/${anime.anime_id}` : undefined,
        lastDownload: formatTimeAgo(anime.last_download_date) || formatDate(anime.last_download_date),
      };
    });

  $: daemonLabel =
    $locale &&
    (status?.status === "running"
      ? m.badge_running()
      : status?.status === "checking"
        ? m.badge_checking()
        : m.badge_stopped());

  // Rodando e verificando são estados "vivos" — só eles ganham o PulseDot (spec §6 restringe o
  // pulso a indicadores de vida; parado é um ponto estático).
  $: daemonAlive = status?.status === "running" || status?.status === "checking";

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === "desc" ? "asc" : "desc";
    } else {
      sortKey = key;
      sortDir = key === "name" ? "asc" : "desc";
    }
  }

  function sortHint(key: SortKey): string {
    if (sortKey !== key) return "";
    return sortDir === "asc" ? m.status_sort_asc() : m.status_sort_desc();
  }

  let wsClient: WebSocketClient | null = null;
  let animesPollInterval: ReturnType<typeof setInterval> | null = null;
  let tickInterval: ReturnType<typeof setInterval> | null = null;
  let torrentsPollInterval: ReturnType<typeof setInterval> | null = null;

  async function loadAnimes() {
    try {
      animes = await getAnimes();
    } catch (err) {
      console.error("Failed to load animes:", err);
    }
  }

  async function loadTorrents() {
    try {
      torrents = await getTorrents();
      torrentsStale = false;
      // spec §8: alimentar o histórico SÓ em poll bem-sucedido. Não existe "repetir a última
      // amostra" — o sparkline congelar é o comportamento correto, fingir stream contínuo não.
      speedHistory.push(totalSpeeds(torrents).download);
      stallTracker.sync(torrents, Date.now());
    } catch (err) {
      torrentsStale = true;
      console.error("Failed to load torrents:", err);
    }
  }

  async function loadInitialData() {
    try {
      loading = true;
      const [statusData, animesData, configData] = await Promise.all([
        getStatus(),
        getAnimes(),
        getConfig(),
      ]);
      status = statusData;
      animes = animesData;
      checkInterval = configData.check_interval;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load data");
    } finally {
      loading = false;
    }
  }

  function handleWebSocketStatus(statusValue: string, lastCheck: string, hasError: boolean) {
    const previousStatus = status?.status;
    if (status) {
      status = { ...status, status: statusValue, last_check: lastCheck, has_error: hasError };
    } else {
      status = { status: statusValue, last_check: lastCheck, has_error: hasError, version: "", disk_total: 0, disk_free: 0 };
    }
    if (previousStatus !== "running" && statusValue === "running") {
      loadAnimes();
    }
  }

  async function handleStart() {
    try {
      actionLoading = true;
      await startDaemon();
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to start daemon");
    } finally {
      actionLoading = false;
    }
  }

  async function handleStop() {
    try {
      actionLoading = true;
      await stopDaemon();
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to stop daemon");
    } finally {
      actionLoading = false;
    }
  }

  async function handleCheck() {
    try {
      actionLoading = true;
      await triggerCheck();
      await loadAnimes();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to trigger check");
    } finally {
      actionLoading = false;
    }
  }

  function formatDate(dateString: string) {
    if (!dateString) return m.common_never();
    const date = new Date(dateString);
    // Datas anteriores a 2010 são o zero-value do Go serializado, não uma data real — o
    // formatDate do domínio só sabe rejeitar datas inválidas, esta regra é do backend.
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return m.common_never();
    return formatDateDomain(date, fmtLocale);
  }

  function formatTimeAgo(dateString: string): string {
    if (!dateString) return "";
    const date = new Date(dateString);
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return "";
    const diffMs = now - date.getTime();
    const diffSeconds = Math.floor(diffMs / 1000);
    const diffMinutes = Math.floor(diffSeconds / 60);
    const diffHours = Math.floor(diffMinutes / 60);
    const diffDays = Math.floor(diffHours / 24);
    const rtf = new Intl.RelativeTimeFormat($locale, { numeric: "auto" });
    if (diffSeconds < 60) return rtf.format(-diffSeconds, "second");
    if (diffMinutes < 60) return rtf.format(-diffMinutes, "minute");
    if (diffHours < 24) return rtf.format(-diffHours, "hour");
    return rtf.format(-diffDays, "day");
  }

  function ringMeta(t: TorrentInfo): string {
    const eta = formatEta(t.eta_seconds, fmtLocale);
    return t.episode_number === null
      ? m.status_hero_ring_meta_batch({ eta })
      : m.status_hero_ring_meta({ episode: t.episode_number, eta });
  }

  onMount(() => {
    loadInitialData();
    loadTorrents();
    torrentsPollInterval = setInterval(loadTorrents, TORRENTS_POLL_MS);
    wsClient = new WebSocketClient();
    wsClient.connect(handleWebSocketStatus, (state) => wsConnectionState.set(state));
    animesPollInterval = setInterval(loadAnimes, 30000);
    tickInterval = setInterval(() => { now = Date.now(); }, 1000);
  });

  onDestroy(() => {
    wsClient?.disconnect();
    wsClient = null;
    if (animesPollInterval) clearInterval(animesPollInterval);
    if (tickInterval) clearInterval(tickInterval);
    if (torrentsPollInterval) clearInterval(torrentsPollInterval);
    // Esta tela é dona do poll que alimenta o histórico; sem o reset o sparkline voltaria a
    // exibir amostras de uma visita anterior como se fossem contíguas às novas.
    speedHistory.reset();
  });

  // Grid compartilhado pelo cabeçalho e pelas linhas da lista (desktop). Definido uma vez para
  // que cabeçalho e conteúdo não possam sair de alinhamento.
  //
  // As larguras fixas foram remedidas depois do aumento da escala tipográfica — cada uma é o
  // texto mais longo que a coluna pode receber, no idioma mais largo, arredondado para cima:
  //   estado    190px — chip "Downloading ep. 128 · 100%" = 183px (text-caption + px-2 + borda)
  //   progresso 290px — legenda "128 watched · 128 downloaded · 128 released of 128" = 287px;
  //                     é uma frase, não um número, e abaixo disso quebra em duas linhas e a
  //                     altura da linha passa a oscilar conforme os valores
  //   último    150px — cabeçalho "ÚLTIMO DOWNLOAD" em text-mono-label (mono + tracking .12em)
  // A soma das colunas fixas + gaps + padding é ~754px, e é por isso que a tabela só aparece a
  // partir de `lg` (1024px): em `md` (768px) sobrariam ~628px e as colunas vazariam da tela.
  // Abaixo disso entram os cards empilhados.
  const LIST_GRID = "grid grid-cols-[44px_minmax(0,1fr)_190px_minmax(290px,340px)_150px] items-center gap-3";
</script>

<div class="space-y-4.5">
  <!-- Header: título, pílula do daemon e as ações do daemon (spec §9.1.1) -->
  <div class="flex flex-wrap items-center gap-3">
    <div class="min-w-0 flex-1">
      <h1 class="text-screen-title text-heading">{T && T.title}</h1>
      <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
    </div>

    {#if status}
      <div
        class="inline-flex items-center gap-2 rounded-pill border px-3 py-1.5 {daemonAlive
          ? 'border-ok-tint/28 bg-ok-tint/12'
          : 'border-default bg-control'}"
      >
        {#if daemonAlive}
          <PulseDot variant={status.status === 'checking' ? 'warn' : 'ok'} size={7} />
        {:else}
          <span class="inline-block h-[7px] w-[7px] shrink-0 rounded-full bg-danger" aria-hidden="true"></span>
        {/if}
        <span class="text-copy {daemonAlive ? 'text-ok' : 'text-body'}">{daemonLabel}</span>
        <span class="h-3.5 w-px bg-default" aria-hidden="true"></span>
        <span class="font-mono text-[12px] text-subtle">
          {formatTimeAgo(status.last_check) || (T && T.never)}
        </span>
      </div>

      <div class="flex shrink-0 gap-2">
        <Button
          variant="ghost"
          on:click={handleCheck}
          disabled={status.status === "checking" || actionLoading}
        >
          {status.status === "checking" ? (T && T.checking) : (T && T.forceCheck)}
        </Button>
        {#if status.status === "stopped"}
          <!-- O verde sólido é escrito inline, não como variante do Button, de propósito: a
               regra de composição do spec §4.1 reserva o preenchimento verde EXCLUSIVAMENTE
               para "Iniciar daemon". Uma variante `ok` no primitivo seria um convite a
               reutilizá-la em outra tela e quebrar a regra de "no máximo um acento sólido". -->
          <button
            type="button"
            class="inline-flex items-center justify-center rounded-control bg-ok px-3 py-1.5 text-copy font-semibold text-on-ok transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            on:click={handleStart}
            disabled={actionLoading}
          >
            {actionLoading ? (T && T.starting) : (T && T.start)}
          </button>
        {:else}
          <Button variant="warn" on:click={handleStop} disabled={actionLoading}>
            {actionLoading ? (T && T.stopping) : (T && T.stop)}
          </Button>
        {/if}
      </div>
    {/if}
  </div>

  {#if loading}
    <Loading message="Loading status..." />
  {:else if status}
    {#if status.has_error && status.status !== "checking"}
      <div
        role="alert"
        class="flex items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
      >
        {T && T.errorAlert}
      </div>
    {/if}

    <div class="grid gap-3.5 lg:grid-cols-[1.15fr_1fr]">
      <!-- Card herói: velocidade agregada + sparkline + um anel por download ativo.
           `min-w-0` NÃO é decorativo: a faixa de downloads ativos abaixo é larga (um bloco de
           ~190px por torrent) e um item de grid tem `min-width: auto`, ou seja, seu piso é o
           tamanho intrínseco do conteúdo. Sem o override o track `1.15fr` estoura — a section
           cresce até o max-content (medido: 2494px num grid de 1280px), o `overflow-x-auto` da
           faixa nunca chega a ativar (largura == scrollWidth) e a coluna da direita é empurrada
           para fora da tela. Com `min-w-0` a section volta a caber no track e a faixa rola. -->
      <section class="min-w-0 rounded-card border border-default bg-card p-4.5">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <p class="font-mono text-mono-label uppercase text-subtle">{T && T.heroSpeedLabel}</p>
            <p class="mt-1.5 flex items-baseline gap-1.5">
              <span class="font-mono text-hero-number text-heading">{heroSpeed.value}</span>
              <span class="text-card-title text-tertiary">{heroSpeed.unit}</span>
            </p>
            <p class="mt-1 text-caption text-subtle">
              {$locale && m.status_hero_summary({
                downloading: activeDownloads.length,
                seeding: seedingCount,
                upload: formatSpeed(speeds.upload, fmtLocale),
              })}
            </p>
          </div>
          <Sparkline values={$speedHistory} variant="accent" label={T && T.heroSpeedLabel} />
        </div>

        <div class="my-3.5 border-t border-divider"></div>

        <p class="font-mono text-mono-label uppercase text-subtle">{T && T.activeDownloads}</p>
        {#if activeDownloads.length > 0}
          <div class="mt-2.5 flex gap-4 overflow-x-auto pb-1">
            {#each activeDownloads as t (t.hash)}
              <div class="flex shrink-0 items-center gap-2.5">
                <ProgressRing value={t.progress} label={t.anime_name ?? t.name} />
                <div class="min-w-0">
                  <p class="max-w-[150px] truncate text-copy text-heading">{t.anime_name ?? t.name}</p>
                  <p class="font-mono text-[12px] text-subtle">{$locale && ringMeta(t)}</p>
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <p class="mt-2 text-caption text-subtle">{T && T.heroNoDownloads}</p>
        {/if}

        <!-- spec §8: a UI declara de onde vem o progresso. O ponto fica âmbar quando o último
             poll falhou, e o texto passa a dizer que os valores estão congelados. -->
        <p class="mt-3 flex items-center gap-1.5 font-mono text-[12px] {torrentsStale ? 'text-warn' : 'text-subtle'}">
          <span
            class="inline-block h-1.5 w-1.5 shrink-0 rounded-full {torrentsStale ? 'bg-warn' : 'bg-neutral'}"
            aria-hidden="true"
          ></span>
          {torrentsStale ? (T && T.staleNote) : (T && T.pollingNote)}
        </p>
      </section>

      <!-- Coluna direita: resumo da biblioteca + disco/próxima verificação -->
      <div class="flex min-w-0 flex-col gap-3.5">
        <section class="rounded-card border border-default bg-card p-4.5">
          <p class="font-mono text-mono-label uppercase text-subtle">{T && T.cardLibrary}</p>
          <p class="mt-1.5 font-mono text-card-number text-heading">
            {$locale && m.status_animes_count({ count: animes.length })}
          </p>
          <div class="mt-3">
            <TripleProgressBar
              watched={libraryTotals.watched}
              downloaded={libraryTotals.downloaded}
              released={libraryTotals.released}
              total={libraryTotals.total}
              legend={$locale ? legendFor(libraryTotals) : ""}
            />
          </div>
        </section>

        <div class="grid grid-cols-2 gap-3.5">
          <section class="rounded-card border border-default bg-card p-4.5">
            <p class="font-mono text-mono-label uppercase text-subtle">{T && T.cardDisk}</p>
            {#if status.disk_total > 0}
              <p class="mt-1.5 font-mono text-card-number {diskSpaceLow ? 'text-danger' : 'text-heading'}">
                {formatBytes(status.disk_free, fmtLocale)}
              </p>
              <p class="mt-1 text-caption text-subtle">
                {$locale && m.status_disk_free_of_total({ total: formatBytes(status.disk_total, fmtLocale) })}
              </p>
            {:else}
              <p class="mt-1.5 font-mono text-card-number text-subtle">—</p>
            {/if}
          </section>

          <section class="rounded-card border border-default bg-card p-4.5">
            <p class="font-mono text-mono-label uppercase text-subtle">{T && T.cardNextCheck}</p>
            <p class="mt-1.5 font-mono text-card-number text-heading">
              {#if status.status === "stopped"}
                <span class="text-subtle">—</span>
              {:else if status.status === "checking"}
                <span class="text-warn text-card-title">{T && T.checking}</span>
              {:else if nextCheckIn}
                {nextCheckIn}
              {:else}
                <span class="text-subtle">—</span>
              {/if}
            </p>
          </section>
        </div>
      </div>
    </div>

    <!-- Lista de animes -->
    <section class="rounded-card border border-default bg-card">
      <!-- Toolbar. `shrink-0` no container é o que impede a faixa de filtros de colapsar
           quando a lista abaixo cresce (spec §9.1, nota de mobile). -->
      <div class="flex shrink-0 flex-col gap-3 border-b border-divider p-4 sm:flex-row sm:items-center">
        <h2 class="flex-1 text-card-title text-heading">{T && T.animesHeader}</h2>

        {#if search || filterUnwatched}
          <span class="whitespace-nowrap font-mono text-[12px] text-subtle">
            {$locale && m.status_x_of_y({ shown: filteredAnimes.length, total: animes.length })}
          </span>
        {/if}

        <!-- No mobile a busca ocupa a linha inteira e os filtros ficam numa faixa própria
             logo abaixo; empilhar os dois na MESMA faixa rolável empurraria o chip de filtro
             para fora da tela, sem nenhuma dica de que ele está lá. -->
        <div class="flex shrink-0 flex-col gap-2 sm:flex-row sm:items-center">
          <label
            class="flex w-full shrink-0 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-1.5 sm:w-64"
          >
            <Search size={16} strokeWidth={2} class="shrink-0 text-subtle" />
            <input
              type="text"
              placeholder={(T && T.searchPlaceholder) || ""}
              bind:value={search}
              class="w-full min-w-0 bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
            />
            {#if search}
              <button
                type="button"
                class="shrink-0 text-subtle hover:text-body"
                aria-label={T && T.clearSearch}
                on:click={() => (search = "")}
              >
                <X size={14} strokeWidth={2} />
              </button>
            {/if}
          </label>

          <!-- Faixa rolável: hoje só tem um filtro, mas é ela que absorve os próximos sem
               espremer a busca (spec §9.1, nota de mobile). -->
          <div class="flex items-center gap-2 overflow-x-auto">
            <button
              type="button"
              class="inline-flex shrink-0 items-center gap-1.5 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {filterUnwatched
                ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
                : 'border-default bg-control text-subtle hover:text-body'}"
              aria-pressed={filterUnwatched}
              on:click={() => (filterUnwatched = !filterUnwatched)}
            >
              <Eye size={14} strokeWidth={2} />
              {T && T.filterUnwatched}
            </button>
          </div>
        </div>
      </div>

      {#if animes.length === 0}
        <div class="flex flex-col items-center gap-3 py-12 text-center">
          <p class="text-card-title text-body">{T && T.emptyTitle}</p>
          <p class="text-caption text-subtle">{T && T.emptyDesc}</p>
          <a
            href="#/config"
            class="mt-1 inline-flex items-center rounded-control bg-accent px-3 py-1.5 text-copy font-semibold text-on-accent transition-opacity hover:opacity-90"
          >
            {T && T.goToConfig}
          </a>
        </div>
      {:else if filteredAnimes.length === 0}
        <div class="py-10 text-center">
          <p class="text-copy text-subtle">{$locale && m.status_no_results({ search })}</p>
        </div>
      {:else}
        <!-- Desktop: grid de linhas-link. Não é <table> de propósito — com a linha inteira
             clicável, um <a> por linha dá o alvo grande sem precisar de handler de clique com
             role/tabindex/keydown manuais numa <tr>. -->
        <div class="hidden lg:block">
          <div class="{LIST_GRID} border-b border-divider px-4 py-2.5">
            <span></span>
            <button
              type="button"
              class="flex items-center gap-1 text-left font-mono text-mono-label uppercase text-subtle transition-colors hover:text-body"
              on:click={() => handleSort("name")}
            >
              {T && T.colName}
              <span class={sortKey === "name" ? "text-accent" : "opacity-30"} aria-hidden="true">
                {sortKey === "name" && sortDir === "asc" ? "▲" : "▼"}
              </span>
              <span class="sr-only">{$locale && sortHint("name")}</span>
            </button>
            <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colState}</span>
            <button
              type="button"
              class="flex items-center gap-1 text-left font-mono text-mono-label uppercase text-subtle transition-colors hover:text-body"
              on:click={() => handleSort("episodes_watched")}
            >
              {T && T.colProgress}
              <span class={sortKey === "episodes_watched" ? "text-accent" : "opacity-30"} aria-hidden="true">
                {sortKey === "episodes_watched" && sortDir === "asc" ? "▲" : "▼"}
              </span>
              <span class="sr-only">{$locale && sortHint("episodes_watched")}</span>
            </button>
            <button
              type="button"
              class="flex items-center gap-1 text-left font-mono text-mono-label uppercase text-subtle transition-colors hover:text-body"
              on:click={() => handleSort("last_download_date")}
            >
              {T && T.colLastDownload}
              <span class={sortKey === "last_download_date" ? "text-accent" : "opacity-30"} aria-hidden="true">
                {sortKey === "last_download_date" && sortDir === "asc" ? "▲" : "▼"}
              </span>
              <span class="sr-only">{$locale && sortHint("last_download_date")}</span>
            </button>
          </div>

          {#each rows || [] as row (row.anime.anime_id || row.anime.name)}
            <svelte:element
              this={row.href ? "a" : "div"}
              href={row.href}
              class="{LIST_GRID} border-b border-divider px-4 py-3 transition-colors last:border-b-0 {row.href
                ? 'hover:bg-control'
                : ''}"
            >
              <!-- Blacklist esmaece capa e título (spec §9.1), não a linha inteira: o chip
                   "Na blacklist" precisa continuar legível para explicar o porquê. -->
              <div class="h-11 w-8 overflow-hidden {row.chip.dimmed ? 'opacity-45' : ''}">
                <Cover src={row.anime.cover_image} alt={row.anime.name} />
              </div>
              <span class="truncate text-copy text-heading {row.chip.dimmed ? 'opacity-45' : ''}" title={row.anime.name}>
                {row.anime.name}
              </span>
              <span>
                <Chip variant={row.chip.variant} dimmed={row.chip.dimmed}>{row.chipText}</Chip>
              </span>
              <TripleProgressBar
                watched={row.breakdown.watched}
                downloaded={row.breakdown.downloaded}
                released={row.breakdown.released}
                total={row.breakdown.total}
                legend={row.legend}
              />
              <span class="flex items-center justify-between gap-1 font-mono text-[12px] text-subtle">
                {row.lastDownload}
                {#if row.href}
                  <ChevronRight size={14} strokeWidth={2} class="shrink-0" />
                {/if}
              </span>
            </svelte:element>
          {/each}
        </div>

        <!-- Mobile/tablet: os mesmos dados empilhados em cards (ver a nota de larguras em LIST_GRID
             sobre por que o corte é em `lg` e não em `md`) -->
        <div class="space-y-2 p-3 lg:hidden">
          {#each rows || [] as row (row.anime.anime_id || row.anime.name)}
            <svelte:element
              this={row.href ? "a" : "div"}
              href={row.href}
              class="block rounded-field border border-default bg-surface p-3 {row.href ? 'active:opacity-80' : ''}"
            >
              <div class="flex gap-3">
                <div class="h-16 w-12 shrink-0 overflow-hidden {row.chip.dimmed ? 'opacity-45' : ''}">
                  <Cover src={row.anime.cover_image} alt={row.anime.name} />
                </div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-copy text-heading {row.chip.dimmed ? 'opacity-45' : ''}">
                    {row.anime.name}
                  </p>
                  <div class="mt-1.5">
                    <Chip variant={row.chip.variant} dimmed={row.chip.dimmed}>{row.chipText}</Chip>
                  </div>
                  <div class="mt-2">
                    <TripleProgressBar
                      watched={row.breakdown.watched}
                      downloaded={row.breakdown.downloaded}
                      released={row.breakdown.released}
                      total={row.breakdown.total}
                      legend={row.legend}
                    />
                  </div>
                  <p class="mt-1.5 font-mono text-[12px] text-subtle">{row.lastDownload}</p>
                </div>
              </div>
            </svelte:element>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>
