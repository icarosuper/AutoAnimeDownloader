<script lang="ts">
  // Downloads — spec §9.3 (Fase 5). A tabela plana vira um accordion por anime: cabeçalho de
  // grupo com progresso agregado e ações em lote, linhas de torrent recuadas dentro dele.
  //
  // Tudo que a tela já fazia continua: busca, ordenação, poda da seleção ao visível, `busy`
  // por hash — e, principalmente, o ESTADO DA VIEW NA URL, que agora carrega também quais
  // grupos estão recolhidos (ver o comentário de `ViewState` em torrentFilters.ts).
  import { onMount, onDestroy } from "svelte";
  import { querystring, replace } from "svelte-spa-router";
  import { ChevronDown, ChevronsUp, Pause, Play, RefreshCw, Trash2 } from "@lucide/svelte";
  import {
    getAnimes,
    getTorrents,
    pauseTorrent,
    resumeTorrent,
    prioritizeTorrent,
    prioritizeTorrents,
    announceTorrent,
    deleteTorrent,
    removeStandaloneAnime,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import {
    filterTorrents,
    sortTorrents,
    groupTorrents,
    isProblemTorrent,
    prioritizeOrder,
    selectionPrioritizeOrder,
    applyFilterPreset,
    activeFilterPreset,
    encodeViewState,
    decodeViewState,
    DEFAULT_VIEW_STATE,
    DOWNLOADING_SLUGS,
    type FilterPreset,
    type ViewState,
    type SortKey,
  } from "../lib/utils/torrentFilters.js";
  import { statusLabel } from "../lib/utils/torrentStatus.js";
  import { formatBytes, formatEta, formatPercent, formatSpeed, type FormatLocale } from "../lib/domain/format.js";
  import Loading from "../components/Loading.svelte";
  import DownloadsToolbar from "../components/DownloadsToolbar.svelte";
  import TorrentDeleteDialog from "../components/TorrentDeleteDialog.svelte";
  import Button from "../components/ui/Button.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import ProgressBar from "../components/ui/ProgressBar.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { wsConnectionState } from "../lib/stores/wsState.js";

  const POLL_MS = 2000;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

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
    colHash: m.downloads_col_hash(),
    colStatus: m.downloads_col_status(),
    colEpisode: m.downloads_col_episode(),
    sortBy: m.downloads_sort_by(),
    pause: m.downloads_pause(),
    resume: m.downloads_resume(),
    announce: m.downloads_announce(),
    prioritize: m.downloads_prioritize(),
    delete: m.downloads_delete(),
    bulkPrioritize: m.downloads_bulk_prioritize(),
    batch: m.downloads_batch(),
    wsBanner: m.downloads_ws_banner(),
    wsReconnect: m.downloads_ws_reconnect(),
    pollingNote: m.status_polling_note({ seconds: POLL_MS / 1000 }),
  };

  let torrents: TorrentInfo[] = [];
  let loading = true;
  // Hashes com ação em voo: desabilitam os botões daquela linha sem congelar a tabela toda.
  let busy = new Set<string>();

  // Capas por anime_id. Buscadas UMA vez (não entram no polling de 2s) — são dado estático de
  // apresentação; se a chamada falhar, `Cover` cai no fallback hachurado e a tela segue.
  let coversByAnimeId = new Map<number, string | undefined>();

  // Estado da view (busca/filtro/ordenação/grupos recolhidos) vive na URL, não em localStorage
  // — veja encodeViewState/decodeViewState em torrentFilters.ts.
  let viewState: ViewState = DEFAULT_VIEW_STATE;

  // Seleção em massa: Set de hashes, sempre podado para o que está visível (pós-filtro).
  let selected = new Set<string>();
  let bulkBusy = false;

  // Diálogo de exclusão — serve tanto para 1 linha quanto para o lote selecionado.
  let deleteDialogOpen = false;
  let pendingDeleteHashes: string[] = [];
  let pendingDeleteName = "";
  // Preenchido só quando a exclusão é do grupo inteiro (o botão do cabeçalho do anime).
  let pendingDeleteAnimeId: number | undefined;
  // Ids dos avulsos, do mesmo GET /animes das capas: para o avulso o 2º check do diálogo deixa
  // de acompanhar o anime em vez de só bloquear os episódios.
  let standaloneAnimeIds = new Set<number>();

  // 'queued' aceita pausar: com `paused` explícito na fila, pausar um enfileirado quer dizer
  // "não inicie sozinho quando o slot vagar" — antes disso, a única forma de impedir que um
  // episódio começasse era deletar o torrent.
  function canPause(t: TorrentInfo): boolean {
    return t.status !== "stopped" && t.status !== "stopping";
  }
  // 'queued' aceita retomar: com limite de downloads simultâneos, retomar significa "volta
  // para o FIM da fila", então num torrent já enfileirado o botão o desprioriza — que é
  // exatamente o oposto do Priorizar ao lado, e por isso os dois convivem.
  function canResume(t: TorrentInfo): boolean {
    return t.status === "stopped" || t.status === "queued";
  }
  // Priorizar é furar a fila. Não faz sentido num torrent que já terminou (o backend
  // devolve erro) nem num que já está baixando.
  function canPrioritize(t: TorrentInfo): boolean {
    return !t.completed && canResume(t);
  }

  // Contagens das pills: derivadas da lista só com a BUSCA aplicada, nunca com o filtro atual.
  // Se elas respeitassem o filtro, escolher "Seeding" zeraria as outras três e o usuário
  // perderia justamente a informação que a pill existe para dar ("tem algo ali?").
  $: searchedTorrents = filterTorrents(torrents, { query: viewState.query, statuses: [] });
  $: counts = {
    all: searchedTorrents.length,
    downloading: searchedTorrents.filter((t) => DOWNLOADING_SLUGS.includes(t.status)).length,
    seeding: searchedTorrents.filter((t) => t.status === "seeding").length,
    problems: searchedTorrents.filter(isProblemTorrent).length,
  };

  // Ordena só quando o usuário clica num cabeçalho; 'default' preserva a ordem do backend.
  $: visibleTorrents = sortTorrents(filterTorrents(torrents, viewState), viewState.sort, viewState.dir);
  // A ordem DOS GRUPOS é fixa (problemas → baixando → resto, ver groupTorrents); a ordenação
  // escolhida pelo usuário vale DENTRO de cada grupo. Assim quem ordena por "peers" continua
  // com os grupos quebrados no topo, onde consegue vê-los.
  $: groups = groupTorrents(visibleTorrents);

  $: speeds = visibleTorrents.reduce(
    (acc, t) => ({ download: acc.download + t.download_speed, upload: acc.upload + t.upload_speed }),
    { download: 0, upload: 0 },
  );

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

  function toggleSelectGroup(hashes: string[]) {
    const next = new Set(selected);
    const allIn = hashes.every((h) => next.has(h));
    hashes.forEach((h) => (allIn ? next.delete(h) : next.add(h)));
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

  function applyPreset(preset: FilterPreset) {
    viewState = applyFilterPreset(viewState, preset);
    syncUrl();
  }

  $: activePreset = activeFilterPreset(viewState);

  function clearFilters() {
    viewState = { ...viewState, query: "", statuses: [], problems: false };
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

  function isClosed(key: string): boolean {
    return viewState.closed.includes(key);
  }

  function toggleGroup(key: string) {
    const closed = isClosed(key)
      ? viewState.closed.filter((k) => k !== key)
      : [...viewState.closed, key];
    viewState = { ...viewState, closed };
    syncUrl();
  }

  /** Hash truncado no formato do artboard: `a3f9c1e8…7b2d`. */
  function shortHash(hash: string): string {
    if (hash.length <= 16) return hash;
    return `${hash.slice(0, 8)}…${hash.slice(-4)}`;
  }

  /**
   * Cor da barra agregada do grupo. A primeira versão usava sempre `accent` (salvo problemas),
   * o que pintava de roxo um grupo 100% concluído — lendo como "ainda baixando" ao lado de
   * chips verdes de "Seeding".
   */
  function groupVariant(g: { problemCount: number; downloadingCount: number; torrents: TorrentInfo[] }) {
    if (g.problemCount > 0) return "danger" as const;
    if (g.downloadingCount > 0) return "accent" as const;
    if (g.torrents.every((t) => t.completed)) return "ok" as const;
    return "neutral" as const;
  }

  function statusVariant(t: TorrentInfo): "accent" | "ok" | "warn" | "danger" | "neutral" {
    if (isProblemTorrent(t)) return "danger";
    switch (t.status) {
      case "seeding": return "ok";
      case "downloading": return "accent";
      case "downloading_metadata":
      case "allocating":
      case "verifying": return "warn";
      default: return "neutral";
    }
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

  async function loadCovers() {
    try {
      const animes = await getAnimes();
      coversByAnimeId = new Map(animes.map((a) => [a.anime_id, a.cover_image]));
      standaloneAnimeIds = new Set(animes.filter((a) => a.is_standalone).map((a) => a.anime_id));
    } catch {
      // Dado de apresentação — o accordion funciona igual com o fallback hachurado.
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

  /** Alvos de uma ação em lote: `scope` limita ao grupo; sem ele, é a seleção inteira. */
  function bulkTargets(scope: TorrentInfo[] | null, predicate?: (t: TorrentInfo) => boolean) {
    const pool = scope ?? visibleTorrents.filter((t) => selected.has(t.hash));
    return pool.filter((t) => (predicate ? predicate(t) : true)).map((t) => t.hash);
  }

  async function handleBulkPause(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope, canPause), pauseTorrent);
    reportBulk(result, m.downloads_bulk_paused);
    selected = new Set();
    await load();
  }

  async function handleBulkResume(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope, canResume), resumeTorrent);
    reportBulk(result, m.downloads_bulk_resumed);
    selected = new Set();
    await load();
  }

  /**
   * Priorizar em lote é UMA requisição, não N — N chamadas ao endpoint por hash se atropelam
   * (cada uma passa na frente da anterior) e inverteriam o lote. Por isso não passa por
   * `runBulk`/`reportBulk`: não existe sucesso parcial para contar, e o backend ignora em
   * silêncio o que já completou, então nem dá para verificar o denominador daqui.
   */
  async function handlePrioritize(hashes: string[]) {
    if (hashes.length === 0) {
      toast.info(m.downloads_bulk_none());
      return;
    }
    bulkBusy = true;
    try {
      await prioritizeTorrents(hashes);
      toast.success(m.downloads_bulk_prioritized({ count: hashes.length }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      bulkBusy = false;
    }
    selected = new Set();
    await load();
  }

  async function handleBulkAnnounce(scope: TorrentInfo[] | null = null) {
    const result = await runBulk(bulkTargets(scope), announceTorrent);
    reportBulk(result, m.downloads_bulk_announced);
    selected = new Set();
    await load();
  }

  // `animeId` só vem preenchido quando esta linha é o único torrent do anime (batch, ou grupo de
  // um): aí excluí-la é excluir o anime, e o diálogo precisa do texto/untrack de anime.
  function handleDeleteRow(t: TorrentInfo, animeId?: number) {
    pendingDeleteHashes = [t.hash];
    pendingDeleteName = t.anime_name || t.name;
    pendingDeleteAnimeId = animeId;
    deleteDialogOpen = true;
  }

  function handleBulkDeleteRequest(scope: TorrentInfo[] | null = null, name = "", animeId?: number) {
    pendingDeleteHashes = bulkTargets(scope);
    pendingDeleteName = name;
    pendingDeleteAnimeId = animeId;
    deleteDialogOpen = true;
  }

  async function confirmDelete(e: CustomEvent<{ keepData: boolean; block: boolean }>) {
    const { keepData, block } = e.detail;
    const hashes = pendingDeleteHashes;
    const animeId = pendingDeleteAnimeId;
    pendingDeleteHashes = [];
    pendingDeleteAnimeId = undefined;

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

    // Avulso + exclusão do anime inteiro: o check prometeu parar de acompanhar. Sem isso o
    // registro avulso continuaria e o próximo passe baixaria tudo de novo. delete_episodes=false
    // porque os arquivos já foram tratados acima, conforme o primeiro check.
    if (block && animeId !== undefined && standaloneAnimeIds.has(animeId)) {
      try {
        await removeStandaloneAnime(animeId, false);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : m.downloads_delete_toast_error());
      }
      await loadCovers();
    }

    selected = new Set();
    await load();
  }

  function cancelDelete() {
    pendingDeleteHashes = [];
  }

  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let wsClient: WebSocketClient | null = null;

  // O banner de "sem conexão" (spec §9.3) precisa de um estado de WebSocket que valha para
  // ESTA tela. Antes só Status.svelte abria a conexão, então quem entrasse direto em
  // /downloads via o store parado no valor inicial 'disconnected' e o banner apareceria
  // sempre. A tela abre a própria conexão, com callback de status vazio — Downloads não exibe
  // estado do daemon, só precisa saber se o canal está de pé.
  function connectWs() {
    wsClient?.disconnect();
    wsClient = new WebSocketClient();
    wsClient.connect(
      () => {},
      (state) => wsConnectionState.set(state),
    );
  }

  onMount(() => {
    viewState = decodeViewState($querystring ?? "");
    load();
    loadCovers();
    connectWs();
    // Polling só enquanto a tela está montada: cada snapshot custa um Stats() por torrent,
    // que é round-trip bloqueante para dentro da goroutine de cada um.
    pollInterval = setInterval(load, POLL_MS);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
    wsClient?.disconnect();
    wsClient = null;
  });

  // Grid das linhas de torrent, compartilhado com o cabeçalho de colunas para que os dois não
  // saiam de alinhamento. O recuo de 66px do spec vem do padding-left, não de uma coluna.
  //
  // Só a partir de `lg`: as sete trilhas somam ~740px, então abaixo disso o grid empurrava a
  // PÁGINA INTEIRA para rolagem horizontal e espremia o título do grupo até "D..". No mobile
  // vira `flex-wrap`, e as células fluem em duas ou três linhas dentro da largura disponível.
  //
  // O corte era `md` (768px) e não bastava: descontando o rail e o padding do main sobram ~628px
  // ali, contra os ~740px que as trilhas exigem — a rolagem horizontal que o comentário acima
  // descreve acontecia exatamente na faixa 768–880px. `lg` (1024px) é o primeiro breakpoint que
  // realmente cabe. Mesmo critério da lista de animes em Status.svelte (ver LIST_GRID).
  const ROW_GRID =
    "flex flex-wrap items-center gap-x-3 gap-y-2 lg:grid lg:grid-cols-[28px_56px_128px_120px_minmax(120px,1fr)_150px_104px] lg:items-center lg:gap-3";
  /** Recuo das linhas dentro do grupo — 66px no desktop, só o padding do card no mobile. */
  const ROW_INDENT = "pl-4 pr-4 lg:pl-[66px]";
</script>

<TorrentDeleteDialog
  bind:open={deleteDialogOpen}
  count={pendingDeleteHashes.length || 1}
  name={pendingDeleteName}
  scope={pendingDeleteAnimeId !== undefined ? "anime" : "episode"}
  standalone={pendingDeleteAnimeId !== undefined && standaloneAnimeIds.has(pendingDeleteAnimeId)}
  on:confirm={confirmDelete}
  on:cancel={cancelDelete}
/>

<div class="space-y-4.5">
  <!-- Header: título + barra de resumo de banda.
       `min-w-[240px]` no bloco do título, e não `min-w-0`: `flex-1` é flex-basis 0%, e a quebra
       de linha de um container `flex-wrap` decide pelo tamanho principal HIPOTÉTICO de cada item
       — com basis 0 e min-width 0 o título contribuía ~0, a caixa de velocidade (433px de
       max-content) nunca ia para a linha de baixo e o título ficava com as sobras, quebrando o
       subtítulo em uma palavra por linha (medido: 7 linhas em 500px, 4 em 768px). O min-width
       ENTRA nesse tamanho hipotético, então ele é o que empurra a caixa para a próxima linha —
       em qualquer largura, sem depender de breakpoint (um `sm:flex-col` consertaria 375px e
       deixaria a faixa dos 500–768px igual). -->
  <div class="flex flex-wrap items-center gap-3">
    <div class="min-w-[240px] flex-1">
      <h1 class="text-screen-title text-heading">{T && T.title}</h1>
      <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
    </div>

    <!-- `flex-wrap` aqui dentro também: em 375px o conteúdo da caixa mede ~337px contra ~343px
         disponíveis, então um valor de velocidade mais largo estouraria a linha. -->
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-[10px] border border-default bg-control px-3 py-2">
      <span class="font-mono text-[15px] font-bold text-ok">↓ {formatSpeed(speeds.download, fmtLocale)}</span>
      <span class="h-[14px] w-px bg-default" aria-hidden="true"></span>
      <span class="font-mono text-[15px] font-bold text-neutral">↑ {formatSpeed(speeds.upload, fmtLocale)}</span>
      <!-- Este divisor sai abaixo de `sm`: é exatamente onde a contagem desce para a segunda
           linha da caixa, e um separador no fim da linha, sem nada depois dele, lê como sujeira.
           O `gap-x-3` continua separando os dois blocos quando eles couberem na mesma linha. -->
      <span class="hidden h-[14px] w-px bg-default sm:inline-block" aria-hidden="true"></span>
      <span class="text-caption text-subtle">
        {$locale && m.downloads_summary_counts({ downloading: counts.downloading, seeding: counts.seeding })}
      </span>
    </div>

    <p class="flex items-center gap-1.5 font-mono text-[12px] text-subtle">
      <span class="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-neutral" aria-hidden="true"></span>
      {T && T.pollingNote}
    </p>
  </div>

  <!-- Banner de WebSocket caído — só quando desconectado (spec §9.3/§8) -->
  {#if $wsConnectionState === "disconnected"}
    <div
      role="alert"
      class="flex flex-wrap items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
    >
      <span class="flex-1">{T && T.wsBanner}</span>
      <button type="button" class="underline hover:opacity-80" on:click={connectWs}>
        {T && T.wsReconnect}
      </button>
    </div>
  {/if}

  {#if loading}
    <Loading message="Loading torrents..." />
  {:else if torrents.length === 0}
    <div class="rounded-card border border-default bg-card py-12 text-center">
      <h2 class="text-card-title text-body">{T && T.emptyTitle}</h2>
      <p class="mt-1 text-caption text-subtle">{T && T.emptyDesc}</p>
    </div>
  {:else}
    <section class="rounded-card border border-default bg-card">
      <DownloadsToolbar
        query={viewState.query}
        preset={activePreset}
        {counts}
        selectedCount={selected.size}
        allSelected={allVisibleSelected}
        someSelected={someVisibleSelected}
        {bulkBusy}
        on:search={(e) => setQuery(e.detail)}
        on:presetChange={(e) => applyPreset(e.detail)}
        on:selectAll={toggleSelectAll}
        on:bulkPrioritize={() => handlePrioritize(selectionPrioritizeOrder(selected, visibleTorrents, canPrioritize))}
        on:bulkPause={() => handleBulkPause()}
        on:bulkResume={() => handleBulkResume()}
        on:bulkAnnounce={() => handleBulkAnnounce()}
        on:bulkDelete={() => handleBulkDeleteRequest()}
        on:deselectAll={() => (selected = new Set())}
      />

      {#if visibleTorrents.length === 0}
        <div class="py-12 text-center">
          <h2 class="text-card-title text-body">{T && T.emptyFilteredTitle}</h2>
          <p class="mt-1 text-caption text-subtle">{T && T.emptyFilteredDesc}</p>
          <div class="mt-3 flex justify-center">
            <Button variant="ghost" on:click={clearFilters}>{T && T.clearFilters}</Button>
          </div>
        </div>
      {:else}
        <!-- Ordenações que não têm coluna própria no accordion continuam acessíveis aqui, com
             o mesmo `setSort` — nenhuma chave de ordenação foi perdida no redesenho. -->
        <div class="flex flex-wrap items-center gap-2 border-b border-divider px-4 py-2">
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.sortBy}</span>
          {#each [
            { key: "name" as SortKey, label: T && T.colName },
            { key: "download_speed" as SortKey, label: T && T.colSpeed },
            { key: "eta" as SortKey, label: T && T.colEta },
          ] as opt (opt.key)}
            <button
              type="button"
              aria-pressed={viewState.sort === opt.key}
              on:click={() => setSort(opt.key)}
              class="rounded-pill border px-2.5 py-1 text-caption font-semibold transition-colors {viewState.sort ===
              opt.key
                ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
                : 'border-default bg-control text-subtle hover:text-body'}"
            >
              {opt.label} {sortIndicator(opt.key)}
            </button>
          {/each}
        </div>

        <!-- Cabeçalho de colunas das linhas de torrent. Recuado igual às linhas, e é aqui que
             a ordenação por coluna (preservada do layout anterior) continua vivendo. -->
        <div class="{ROW_GRID} hidden border-b border-divider py-2.5 {ROW_INDENT} lg:grid">
          <span></span>
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colEpisode}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colStatus}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colHash}</span>
          <button
            type="button"
            class="text-left font-mono text-mono-label uppercase text-subtle hover:text-body"
            on:click={() => setSort("progress")}
          >
            {T && T.colProgress} {sortIndicator("progress")}
          </button>
          <button
            type="button"
            class="text-left font-mono text-mono-label uppercase text-subtle hover:text-body"
            on:click={() => setSort("peers")}
          >
            {T && T.colPeers} {sortIndicator("peers")}
          </button>
          <span class="text-right font-mono text-mono-label uppercase text-subtle">{T && T.colActions}</span>
        </div>

        {#each groups as group (group.key)}
          {@const groupHashes = group.torrents.map((t) => t.hash)}
          {@const groupSelected = groupHashes.every((h) => selected.has(h))}
          {@const groupSome = groupHashes.some((h) => selected.has(h)) && !groupSelected}
          {@const closed = isClosed(group.key)}
          <div data-group class="border-b border-divider last:border-b-0">
            <div class="flex flex-wrap items-center gap-3 px-4 py-3">
              <Checkbox
                checked={groupSelected}
                indeterminate={groupSome}
                label={$locale ? m.downloads_group_select({ name: group.name }) : ""}
                labelHidden
                on:change={() => toggleSelectGroup(groupHashes)}
              />

              <div class="h-[46px] w-[34px] shrink-0 overflow-hidden">
                <Cover src={group.animeId !== undefined ? coversByAnimeId.get(group.animeId) : undefined} alt={group.name} />
              </div>

              <!-- O botão cobre título/contagem/barra: alvo grande para expandir, sem aninhar
                   o checkbox e as ações do grupo dentro de outro elemento clicável.
                   `min-w-[180px]`, não `min-w-0`: com o cabeçalho em flex-wrap, uma coluna que
                   encolhe até zero nunca empurra os botões de lote para a linha de baixo — no
                   mobile o nome do grupo virava "B..". -->
              <button
                type="button"
                class="flex min-w-[180px] flex-1 items-center gap-3 text-left"
                aria-expanded={!closed}
                aria-label={$locale ? m.downloads_group_toggle({ name: group.name }) : ""}
                on:click={() => toggleGroup(group.key)}
              >
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="truncate text-[16px] font-extrabold text-heading">{group.name}</span>
                    <Chip variant="neutral">{$locale && m.downloads_group_count({ count: group.torrents.length })}</Chip>
                    {#if group.problemCount > 0}
                      <Chip variant="danger">{$locale && m.downloads_group_problems({ count: group.problemCount })}</Chip>
                    {/if}
                  </div>
                  <div class="mt-1.5 flex items-center gap-2">
                    <div class="min-w-0 flex-1">
                      <ProgressBar
                        value={group.progress}
                        thickness={7}
                        variant={groupVariant(group)}
                        label={$locale ? m.downloads_group_progress({ name: group.name }) : group.name}
                      />
                    </div>
                    <span class="shrink-0 font-mono text-[12px] text-subtle">
                      {formatPercent(group.progress, fmtLocale)}
                    </span>
                    <span class="shrink-0 font-mono text-[12px] text-subtle">
                      ↓ {formatSpeed(group.downloadSpeed, fmtLocale)}
                    </span>
                  </div>
                </div>

                <ChevronDown
                  size={18}
                  strokeWidth={2}
                  class="shrink-0 text-subtle transition-transform {closed ? '-rotate-90' : ''}"
                />
              </button>

              <!-- Ações em lote do grupo, quadrados de 32px.
                   Os tooltips do daisyUI 4 saem do `data-tip` do wrapper (`.tooltip-content` é
                   v5), mesmo padrão do NavRail. `tooltip-left` porque estes botões ficam
                   colados na borda direita do card. O texto é só o RÓTULO DA AÇÃO: o
                   `aria-label` concatena " — {nome}", que no tooltip seria ruído.

                   Grupo de um torrent só (batch, ou anime com um episódio) aberto não ganha barra
                   de lote: ela repetiria pausar/excluir a um palmo dos mesmos botões da linha. As
                   ações da linha são um superconjunto (têm announce e respeitam o estado), e o
                   excluir de lá já usa o escopo do anime quando a linha é o grupo inteiro.
                   Recolhido a barra volta — senão o grupo ficaria sem nenhuma ação. -->
              {#if group.torrents.length > 1 || closed}
              <div class="flex shrink-0 items-center gap-1">
                <div class="tooltip tooltip-left" data-tip={T && T.bulkPrioritize}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label={$locale ? m.downloads_group_prioritize({ name: group.name }) : ""}
                    disabled={bulkBusy}
                    on:click={() => handlePrioritize(prioritizeOrder(group.torrents.filter(canPrioritize)))}
                  >
                    <ChevronsUp size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.pause}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label="{T && T.pause} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkPause(group.torrents)}
                  >
                    <Pause size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.resume}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body"
                    aria-label="{T && T.resume} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkResume(group.torrents)}
                  >
                    <Play size={15} strokeWidth={2} />
                  </button>
                </div>
                <div class="tooltip tooltip-left" data-tip={T && T.delete}>
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-control border border-danger-tint/32 text-danger transition-colors hover:bg-danger-tint/12"
                    aria-label="{T && T.delete} — {group.name}"
                    disabled={bulkBusy}
                    on:click={() => handleBulkDeleteRequest(group.torrents, group.name, group.animeId)}
                  >
                    <Trash2 size={15} strokeWidth={2} />
                  </button>
                </div>
              </div>
              {/if}
            </div>

            {#if !closed}
              {#each group.torrents as t (t.hash)}
                <div
                  data-torrent-row
                  class="{ROW_GRID} border-t border-divider py-2.5 {ROW_INDENT} {selected.has(t.hash)
                    ? 'bg-accent-tint/[.06]'
                    : ''}"
                >
                  <Checkbox
                    checked={selected.has(t.hash)}
                    label={$locale ? m.downloads_select_row({ name: t.anime_name || t.name }) : ""}
                    labelHidden
                    on:change={() => toggleSelect(t.hash)}
                  />

                  <span class="font-mono text-[13px] font-bold text-heading">
                    {#if t.is_batch}
                      {T && T.batch}
                    {:else if t.episode_number !== null}
                      {t.episode_number}
                    {:else}
                      —
                    {/if}
                  </span>

                  <!-- A posição na fila entra no chip que já existe, sem coluna nova. Ela NÃO
                       entra em `statusLabel()`: o filtro de status da toolbar chama a mesma
                       função, e "Na fila #7" não faria sentido nenhum ali.
                       Os números saem não-contíguos dentro de um grupo (#2, #5, #6) quando
                       outro anime está intercalado na fila — é justamente o que comunica que
                       a fila é global; renumerar por grupo destruiria essa informação. -->
                  <span>
                    <Chip variant={statusVariant(t)}>
                      {$locale &&
                        (t.queue_position > 0
                          ? m.downloads_queue_position({ position: t.queue_position })
                          : statusLabel(t.status))}
                    </Chip>
                  </span>

                  <span class="truncate font-mono text-[12px] text-subtle" title={t.hash}>{shortHash(t.hash)}</span>

                  <div class="w-full min-w-0 lg:w-auto">
                    <!-- bytes_total fica em 0 até a metadata chegar; sem ela o percentual não
                         significa nada ainda, então a barra fica no trilho vazio. -->
                    <ProgressBar
                      value={t.bytes_total > 0 ? t.progress : 0}
                      thickness={4}
                      variant={statusVariant(t)}
                      label={t.name}
                    />
                    <p class="mt-1 truncate font-mono text-[12px] text-subtle">
                      {#if t.bytes_total === 0}
                        —
                      {:else if t.bytes_completed === 0 && t.progress > 0}
                        <!-- Pausar zera bytes_completed (o rain libera os dados das peças) mas
                             `progress` continua reportando a fração real. Mostrar "0 B / 1,4 GB"
                             ao lado de uma barra não-vazia leria como contradição. -->
                        {formatPercent(t.progress, fmtLocale)}
                      {:else}
                        {formatPercent(t.progress, fmtLocale)} · {formatBytes(t.bytes_completed, fmtLocale)} / {formatBytes(t.bytes_total, fmtLocale)}
                      {/if}
                    </p>
                  </div>

                  <div class="min-w-0 font-mono text-[12px] text-subtle">
                    <p class="truncate">{$locale && m.downloads_peers_count({ count: t.peers_total })} · {formatEta(t.eta_seconds, fmtLocale)}</p>
                    <p class="truncate">↓ {formatSpeed(t.download_speed, fmtLocale)} · ↑ {formatSpeed(t.upload_speed, fmtLocale)}</p>
                  </div>

                  <!-- Ações individuais, quadrados de 28px.
                       Play e Pause são dois `{#if}` INDEPENDENTES, não os dois ramos de um
                       if/else: um torrent `queued` tem `canResume === true`, então no if/else
                       ele caía no ramo do Play e o Pause nunca chegava ao DOM. Relaxar
                       `canPause` sozinho não renderizaria nada. Um enfileirado fica com os três,
                       cada um numa direção: Priorizar sobe, Play desce para o fim da fila,
                       Pause tira da rotação. -->
                  <div class="flex items-center justify-end gap-1">
                    {#if canPrioritize(t)}
                      <div class="tooltip" data-tip={T && T.prioritize}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.prioritize} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, prioritizeTorrent)}
                        >
                          <ChevronsUp size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    {#if canResume(t)}
                      <div class="tooltip" data-tip={T && T.resume}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.resume} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, resumeTorrent)}
                        >
                          <Play size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    {#if canPause(t)}
                      <div class="tooltip" data-tip={T && T.pause}>
                        <button
                          type="button"
                          class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                          aria-label="{T && T.pause} — {t.name}"
                          disabled={busy.has(t.hash)}
                          on:click={() => runAction(t.hash, pauseTorrent)}
                        >
                          <Pause size={14} strokeWidth={2} />
                        </button>
                      </div>
                    {/if}
                    <div class="tooltip" data-tip={T && T.announce}>
                      <button
                        type="button"
                        class="flex h-7 w-7 items-center justify-center rounded-control border border-default text-subtle transition-colors hover:bg-control hover:text-body disabled:opacity-50"
                        aria-label="{T && T.announce} — {t.name}"
                        disabled={busy.has(t.hash)}
                        on:click={() => runAction(t.hash, announceTorrent)}
                      >
                        <RefreshCw size={14} strokeWidth={2} />
                      </button>
                    </div>
                    <div class="tooltip tooltip-left" data-tip={T && T.delete}>
                      <button
                        type="button"
                        class="flex h-7 w-7 items-center justify-center rounded-control border border-danger-tint/32 text-danger transition-colors hover:bg-danger-tint/12 disabled:opacity-50"
                        aria-label="{T && T.delete} — {t.name}"
                        disabled={busy.has(t.hash)}
                        on:click={() => handleDeleteRow(t, group.torrents.length === 1 ? group.animeId : undefined)}
                      >
                        <Trash2 size={14} strokeWidth={2} />
                      </button>
                    </div>
                  </div>
                </div>
              {/each}
            {/if}
          </div>
        {/each}
      {/if}
    </section>
  {/if}
</div>
