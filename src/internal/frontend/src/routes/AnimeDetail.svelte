<script lang="ts">
  // AnimeDetail — spec §9.2 (Fase 4). A tela de maior ganho do redesenho: a versão anterior
  // tinha 971 linhas em boa parte porque a lista de ações estava escrita DUAS VEZES (tabela
  // desktop + cards mobile), com os mesmos cinco botões só-ícone cujo `title` era a única
  // pista textual e que trocavam de posição entre linhas conforme apareciam e sumiam.
  //
  // Agora existe UMA definição de ações — `episodeActions()` (lib/domain/), dado puro — que
  // desktop e mobile renderizam pelo mesmo `ActionMenu`. A ação principal tem texto e fica
  // sempre na mesma coluna; o resto vive no `⋯`, também rotulado.
  import { onDestroy } from "svelte";
  import { ChevronDown, ChevronRight } from "@lucide/svelte";
  import {
    getAnimeDetail,
    getAnimes,
    getTorrents,
    downloadEpisode,
    deleteEpisode,
    releaseEpisode,
    redownloadEpisode,
    replaceEpisodeWithMagnet,
    replaceAnimeWithMagnet,
    updateAnimeSettings,
    removeStandaloneAnime,
    deleteTorrent,
    getLastCheck,
    type AnimeDetailResponse,
    type AnimeEpisodeInfo,
    type AnimeInfo,
    type TorrentInfo,
    type Issue,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import TorrentDeleteDialog from "../components/TorrentDeleteDialog.svelte";
  import ActionMenu, { type ActionMenuItem } from "../components/ui/ActionMenu.svelte";
  import Button from "../components/ui/Button.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import Chip from "../components/ui/Chip.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import Modal from "../components/ui/Modal.svelte";
  import ProgressBar from "../components/ui/ProgressBar.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";
  import { indexTorrentsByEpisode } from "../lib/utils/torrentsByEpisode.js";
  import { statusLabel } from "../lib/utils/torrentStatus.js";
  import { nextAiringIn } from "../lib/utils/relativeTime.js";
  import { episodeActions, type Action, type EpisodeActionId } from "../lib/domain/episodeActions.js";
  import { deriveAnimeChip, type AnimeChipState } from "../lib/domain/animeState.js";
  import { issueMessage, batchNote } from "../lib/domain/checkIssue.js";
  import {
    formatDate as formatDateDomain,
    formatEta,
    formatPercent,
    formatSpeed,
    type FormatLocale,
  } from "../lib/domain/format.js";
  import { stallTracker } from "../lib/stores/stallTracker.js";

  export let params: { id?: string } = {};

  $: animeId = parseInt(params.id || "0");
  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  let anime: AnimeInfo | null = null;
  let detail: AnimeDetailResponse | null = null;
  let loading = true;
  let actionLoading: Record<number, boolean> = {};
  let confirmOpen = false;
  let pendingDeleteEp: AnimeEpisodeInfo | null = null;
  let confirmRedownloadOpen = false;
  let pendingRedownloadEp: AnimeEpisodeInfo | null = null;

  // Bulk selection state
  let selectedEpisodes: Set<number> = new Set();
  let bulkLoading = false;
  let confirmBulkOpen = false;

  // Deixar de acompanhar (só para avulsos). Mora no header do AnimeDetail porque é onde o
  // usuário já está depois de clicar no anime na lista do Status, e o header já é o lugar das
  // ações de anime — pôr isso na linha do Status custaria uma coluna de ActionMenu que a lista
  // não tem hoje.
  let untrackOpen = false;
  let untrackDeleteFiles = false;
  let untrackLoading = false;

  // Replace with magnet state
  let replaceEpOpen = false;
  let pendingReplaceEp: AnimeEpisodeInfo | null = null;
  let replaceEpMagnet = "";
  let replaceAnimeOpen = false;
  let replaceAnimeMagnet = "";
  let replaceLoading = false;

  // Custom search query state. O handoff esqueceu deste campo; o spec §9.2 pede que ele volte
  // num bloco recolhível logo abaixo do cabeçalho — recolhido por padrão porque é ajuste fino,
  // não o que se vem fazer nesta tela.
  let customSearchQuery = "";
  let searchQuerySaving = false;
  let searchQueryOpen = false;

  // Progresso manual do avulso. Prefill de `anime.episodes_watched`, que já traz o valor salvo
  // (o backend injeta AnimeSettings.progress no MediaList sintético).
  let progressInput = 0;
  let progressSaving = false;
  $: progressInput = anime?.episodes_watched ?? 0;

  async function saveProgress(value: number) {
    progressSaving = true;
    try {
      await updateAnimeSettings(animeId, { progress: value });
      toast.success(m.detail_toast_progress_saved());
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_progress_error());
    } finally {
      progressSaving = false;
    }
  }

  // Live torrent progress, joined onto episodes below. Fetch failures degrade silently:
  // this is accessory data (mirrors the best-effort join the backend already does in
  // handleTorrents), so the page must never break because of it.
  let torrents: TorrentInfo[] = [];
  // Só as entradas deste anime. Mesma chamada do Status, filtrada por anime_id: um endpoint com
  // dois consumidores em vez da mesma informação espalhada por três formas que precisariam
  // concordar entre si.
  let animeIssues: Issue[] = [];
  // Só problema marca episódio: limite não tem "episódios afetados" (o daemon parou de
  // considerar episódios ao atingir a conta).
  $: issueByEpisode = new Map<number, Issue>(
    animeIssues.flatMap((i) => (i.episodes ?? []).map((n) => [n, i] as [number, Issue])),
  );
  let torrentPollTimer: ReturnType<typeof setTimeout> | null = null;
  let torrentPollStarted = false;
  let torrentPollAnimeId: number | null = null;
  // Carimbo do último poll bem-sucedido. `deriveAnimeChip` só usa `now` para o limiar de "sem
  // seeds"; atualizar por tick de relógio faria a tela inteira recalcular de segundo em
  // segundo sem que nenhum dado tivesse mudado.
  let lastPollAt = Date.now();

  // Contagem regressiva do próximo episódio. Ancorada em `lastPollAt` pela mesma razão do
  // comentário acima — o texto é em horas/dias, nada aqui justifica um tick de segundo.
  $: nextAiringWhen = $locale ? nextAiringIn(anime?.next_airing_at, lastPollAt, $locale) : "";

  $: allEpisodes = detail?.episodes ?? [];
  $: torrentsByEpisode = indexTorrentsByEpisode(torrents, allEpisodes);

  // ATENÇÃO ao mexer no inline de progresso lá embaixo: um episódio só ganha `episode_hash`
  // quando o daemon grava o registro salvo — e ele grava no momento em que o torrent é
  // *adicionado*, não quando termina (daemon/episodes.go, daemon/manual_download.go). Como
  // endpoint_anime_episodes.go preenche `is_downloaded` e `episode_hash` no mesmo `if`, vale
  // `episode_hash != "" ⟺ is_downloaded`. Logo a condição do inline não pode exigir
  // `!ep.is_downloaded`: essa combinação nunca ocorre na prática e a barra nunca aparecia.
  // Quem decide é só o torrent ainda estar em voo (`!torrent.completed`).
  $: allSelected = allEpisodes.length > 0 && allEpisodes.every(ep => selectedEpisodes.has(ep.episode_number));
  $: someSelected = selectedEpisodes.size > 0 && !allSelected;

  $: selectedList = allEpisodes.filter(ep => selectedEpisodes.has(ep.episode_number));
  $: canBulkDownload = selectedList.some(ep => ep.is_aired && !ep.is_downloaded);
  $: canBulkDelete = selectedList.some(ep => ep.is_downloaded);
  $: canBulkRelease = selectedList.some(ep => ep.is_manually_managed || ep.is_blocked);

  // Chip do anime no cabeçalho — mesma cascata da lista de Status, mesmo componente.
  $: animeChip = anime ? deriveAnimeChip(anime, torrents, lastPollAt, $stallTracker) : null;

  function toggleSelectAll() {
    if (allSelected) {
      selectedEpisodes = new Set();
    } else {
      selectedEpisodes = new Set(allEpisodes.map(ep => ep.episode_number));
    }
  }

  function toggleEpisode(id: number) {
    const next = new Set(selectedEpisodes);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selectedEpisodes = next;
  }

  function formatDate(dateString: string | undefined) {
    if (!dateString) return m.common_na();
    return formatDateDomain(dateString, fmtLocale);
  }

  function formatTimeUntilAiring(seconds: number, isAired: boolean): string {
    if (isAired || seconds <= 0) return "—";
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  }

  function chipLabel(state: AnimeChipState): string {
    switch (state.key) {
      case "blacklisted": return m.chip_blacklisted();
      case "downloading":
        return state.episodeNumber === undefined
          ? m.chip_downloading_no_episode({ percent: state.percent ?? 0 })
          : m.chip_downloading({ episode: state.episodeNumber, percent: state.percent ?? 0 });
      case "noSeeds": return m.chip_no_seeds();
      case "paused": return m.chip_paused();
      case "awaitingPremiere": return m.chip_awaiting_premiere();
      case "upToDate": return m.chip_up_to_date();
      case "behind": return m.chip_behind({ count: state.behindCount ?? 0 });
    }
  }

  /**
   * Estado de UM episódio, para o `Chip` da coluna "Estado". Um torrent em voo vence tudo —
   * é a informação mais volátil e a única que o usuário está acompanhando ativamente — e o
   * rótulo vem de `statusLabel`, que já traduz os slugs crus do rain (spec §7: estados nunca
   * aparecem como identificador).
   */
  function episodeChip(ep: AnimeEpisodeInfo, torrent: TorrentInfo | undefined) {
    if (torrent && !torrent.completed) {
      return { variant: "accent" as const, label: statusLabel(torrent.status) };
    }
    if (ep.is_blocked) return { variant: "danger" as const, label: m.detail_chip_blocked() };
    if (ep.is_manually_managed) return { variant: "neutral" as const, label: m.detail_chip_manual() };
    if (ep.is_downloaded) return { variant: "ok" as const, label: m.detail_badge_downloaded() };
    if (ep.is_aired) return { variant: "warn" as const, label: m.detail_chip_aired() };
    return { variant: "neutral" as const, label: m.detail_badge_upcoming() };
  }

  /** Nota secundária sob o título: flags do episódio, na forma de frase, não de ícone mudo. */
  function episodeNotes(ep: AnimeEpisodeInfo, issue: Issue | undefined): string {
    const notes: string[] = [];
    if (ep.is_watched) notes.push(m.detail_badge_watched());
    if (ep.is_manually_managed) notes.push(m.detail_flag_no_delete_short());
    if (ep.is_blocked) notes.push(m.detail_flag_no_download_short());
    // A marca do relatório vai na mesma linha das outras notas: é uma frase curta, não um
    // estado ao vivo — deriveAnimeChip fica fora disto de propósito (ver decisions.md).
    if (issue) notes.push(m.detail_lastcheck_episode_note());
    return notes.join(" · ");
  }

  /** Metadado da linha: o dado mais informativo para o estado atual do episódio. */
  function episodeMeta(ep: AnimeEpisodeInfo, torrent: TorrentInfo | undefined): string {
    if (torrent && !torrent.completed) {
      return `${formatPercent(torrent.progress, fmtLocale)} · ↓ ${formatSpeed(torrent.download_speed, fmtLocale)} · ${formatEta(torrent.eta_seconds, fmtLocale)}`;
    }
    if (ep.is_downloaded) return formatDate(ep.download_date);
    if (!ep.is_aired) return formatTimeUntilAiring(ep.time_until_airing, ep.is_aired);
    return "—";
  }

  // `episodeActions` devolve `labelKey`, um identificador — nunca texto. Esta é a única
  // tradução dele na tela, compartilhada pela ação principal e pelos itens do `⋯`.
  const ACTION_LABEL: Record<string, () => string> = {
    download: m.detail_btn_download,
    redownload: m.detail_btn_redownload,
    delete: m.detail_btn_delete,
    release: m.detail_btn_release,
    replace: m.detail_btn_replace,
    watchedHere: m.detail_btn_watched_here,
  };

  function actionLabel(action: Action): string {
    return (ACTION_LABEL[action.labelKey] ?? (() => action.labelKey))();
  }

  function menuItems(actions: Action[]): ActionMenuItem[] {
    return actions.map((a) => ({ id: a.id, label: actionLabel(a), destructive: a.destructive }));
  }

  /**
   * Único ponto de despacho das cinco ações. `delete` e `redownload` continuam passando pelos
   * diálogos de confirmação que já existiam — spec §7 é explícito: exclusão nunca é um clique
   * só, e `episodeActions` marca as duas como `destructive` justamente para sinalizar isso.
   */
  function runAction(id: EpisodeActionId, ep: AnimeEpisodeInfo) {
    switch (id) {
      case "download": return handleDownload(ep);
      case "redownload": return handleRedownload(ep);
      case "delete": return handleDelete(ep);
      case "release": return handleRelease(ep);
      case "replace": return handleReplace(ep);
      case "watchedHere": return saveProgress(ep.episode_number);
    }
  }

  // Uma passada monta tudo que cada linha precisa, para desktop e mobile lerem a MESMA
  // estrutura — é isso que impede as duas cópias de divergirem de novo.
  //
  // Um pack é UMA linha. A regra de detecção é a mesma que o backend já usa em
  // buildTorrentResponse: mais de um episódio no mesmo info hash é batch, diga o que disser a
  // flag. Excluir um episódio de dentro de um pack não libera byte nenhum (o hardlink da
  // biblioteca e a cópia de seed são o mesmo inode) e "Rebaixar" buscaria episódio solto, que
  // numa série longa não existe — então a linha de grupo oferece só o que é verdade para um pack.
  //
  // Nem "Substituir": esse botão chama replaceAnimeWithMagnet, que apaga TODOS os torrents do
  // anime (endpoint_episode_actions.go), não só os do pack clicado — com packs sucessivos
  // (001-100 + 101-200), "Substituir" no primeiro destruiria o segundo em silêncio. O mesmo
  // argumento do comentário acima sobre "Rebaixar" vale aqui: só sobra "Excluir", que já vai por
  // DELETE /torrents/{hash} e opera exatamente na unidade certa.
  //
  // ponytail: buildRows é O(n²) (o .filter() por pack dentro do loop principal) — irrelevante
  // para o tamanho real de um anime, mas se esta tela um dia listar milhares de episódios em
  // dezenas de packs, trocar por um Map<hash, AnimeEpisodeInfo[]> montado em uma passada resolve.
  function buildRows(episodes: AnimeEpisodeInfo[], byEpisode: Map<number, TorrentInfo>) {
    const counts = new Map<string, number>();
    for (const ep of episodes) {
      if (ep.episode_hash) counts.set(ep.episode_hash, (counts.get(ep.episode_hash) ?? 0) + 1);
    }

    // Um pack por hash, com a faixa que ele de fato cobre. A faixa vem do registro (lida do nome
    // do torrent no download), e não do min/max dos episódios salvos: os já assistidos nunca viram
    // registro, então o min/max encolhe o pack — um 01-11 baixado com 5 assistidos virava "6–11".
    // Sem faixa salva (registro antigo) o min/max continua sendo o melhor palpite.
    const packs = new Map<string, { first: number; last: number }>();
    for (const ep of episodes) {
      const hash = ep.episode_hash;
      if (!hash || (counts.get(hash) ?? 0) < 2 || packs.has(hash)) continue;
      const numbers = episodes.filter((e) => e.episode_hash === hash).map((e) => e.episode_number);
      packs.set(hash, {
        first: ep.batch_start || Math.min(...numbers),
        last: ep.batch_end || Math.max(...numbers),
      });
    }

    // O pack engole os episódios de dentro da faixa que não têm torrent próprio — tipicamente os
    // já assistidos, que nunca viraram registro. Eles ESTÃO no pack: o torrent traz os arquivos e
    // o Organize hardlinka o diretório inteiro na biblioteca, então uma linha "Baixar" para eles
    // oferecia um segundo torrent do que já está vindo. Não lançado não é engolido — o pack que
    // está no disco não pode conter o que não foi ao ar.
    const coveringPack = (ep: AnimeEpisodeInfo) => {
      if (ep.episode_hash) return ep.episode_hash;
      if (!ep.is_aired) return undefined;
      for (const [hash, range] of packs) {
        if (ep.episode_number >= range.first && ep.episode_number <= range.last) return hash;
      }
      return undefined;
    };

    const out = [];
    const seenPack = new Set<string>();

    for (const ep of episodes) {
      const hash = coveringPack(ep);
      if (hash && (counts.get(hash) ?? 0) >= 2) {
        if (seenPack.has(hash)) continue;
        seenPack.add(hash);

        const group = episodes.filter((e) => e.episode_hash === hash);
        const torrent = byEpisode.get(group[0].episode_number);
        const { first, last } = packs.get(hash)!;
        out.push({
          kind: "batch" as const,
          key: `pack-${hash}`,
          hash,
          ep: group[0],
          torrent,
          inFlight: !!torrent && !torrent.completed,
          chip: episodeChip(group[0], torrent),
          notes: "",
          meta: episodeMeta(group[0], torrent),
          principal: undefined,
          menu: [{ id: "delete", label: m.detail_btn_delete(), destructive: true as const }],
          title: m.detail_batch_row_title({ first, last }),
          label: `${first}–${last}`,
        });
        continue;
      }

      const torrent = byEpisode.get(ep.episode_number);
      const actions = episodeActions(ep, torrent, { standalone: !!anime?.is_standalone });
      out.push({
        kind: "episode" as const,
        key: `ep-${ep.episode_number}`,
        hash: ep.episode_hash,
        ep,
        torrent,
        inFlight: !!torrent && !torrent.completed,
        chip: episodeChip(ep, torrent),
        notes: episodeNotes(ep, issueByEpisode.get(ep.episode_number)),
        meta: episodeMeta(ep, torrent),
        principal: actions.principal,
        menu: menuItems(actions.menu),
        // Sempre o título localizado. Não existe título de episódio em lugar nenhum: o AniList
        // não o expõe por episódio, e o `episode_name` do registro salvo é o rótulo de log do
        // download ("Anime - Episode 7"), que a API não devolve mais.
        title: m.detail_ep_title({ number: ep.episode_number }),
        label: String(ep.episode_number),
      });
    }

    return out;
  }

  $: rows = $locale && issueByEpisode && buildRows(allEpisodes, torrentsByEpisode);

  // Ação da linha de grupo: só "Excluir", pelo DELETE /torrents/{hash}, que já remove o torrent e
  // TODOS os registros do grupo como unidade. Nem "Rebaixar" (seria alias de Excluir, já que o
  // loop rebusca no ciclo seguinte assim que o espaço volta) nem "Substituir" (destruiria packs
  // irmãos em silêncio — ver o comentário grande em buildRows) fazem sentido aqui, então não há
  // ramificação por id: o único item do menu de pack já é "delete".
  //
  // ponytail: essa ação fica inline aqui, e não em lib/domain/episodeActions.ts, porque hoje só
  // existe UMA linha de pack em UMA tela, e ela é uma constante fixa (não uma classificação de
  // estado como o classify() de episodeActions.ts). Se uma segunda tela precisar da mesma decisão
  // de pack, aí sim vale extrair um packActionSet() exportado de episodeActions.ts.
  let packDeleteOpen = false;
  let pendingPackHash = "";
  let pendingPackLabel = "";

  function openPackDelete(row: { hash?: string; label: string }) {
    pendingPackHash = row.hash ?? "";
    pendingPackLabel = row.label;
    packDeleteOpen = true;
  }

  async function confirmPackDelete(opts: { keepData: boolean; block: boolean }) {
    packDeleteOpen = false;
    try {
      await deleteTorrent(pendingPackHash, opts);
      toast.success(m.detail_toast_pack_deleted({ label: pendingPackLabel }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_del_error());
    }
  }

  async function loadData(id: number) {
    if (!id || id <= 0) {
      loading = false;
      return;
    }

    try {
      loading = true;

      const [detailData, animesData] = await Promise.all([
        getAnimeDetail(id),
        getAnimes(),
      ]);

      detail = detailData;
      anime = animesData.find((a) => a.anime_id === id) ?? null;
      customSearchQuery = detailData.custom_search_query ?? "";
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_load_error());
    } finally {
      loading = false;
    }
  }

  async function handleDownload(ep: AnimeEpisodeInfo) {
    actionLoading = { ...actionLoading, [ep.episode_number]: true };
    try {
      await downloadEpisode(animeId, ep.episode_number);
      toast.success(m.detail_toast_queued({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_dl_error());
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_number]: false };
    }
  }

  function handleDelete(ep: AnimeEpisodeInfo) {
    pendingDeleteEp = ep;
    confirmOpen = true;
  }

  async function confirmDelete() {
    if (!pendingDeleteEp) return;
    const ep = pendingDeleteEp;
    pendingDeleteEp = null;
    actionLoading = { ...actionLoading, [ep.episode_number]: true };
    try {
      await deleteEpisode(animeId, ep.episode_number);
      toast.success(m.detail_toast_deleted({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_del_error());
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_number]: false };
    }
  }

  async function handleRelease(ep: AnimeEpisodeInfo) {
    actionLoading = { ...actionLoading, [ep.episode_number]: true };
    try {
      await releaseEpisode(animeId, ep.episode_number);
      await loadData(animeId);
    } catch (err) {
      console.error("Failed to release episode:", err);
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_number]: false };
    }
  }

  function handleRedownload(ep: AnimeEpisodeInfo) {
    pendingRedownloadEp = ep;
    confirmRedownloadOpen = true;
  }

  async function confirmRedownload() {
    if (!pendingRedownloadEp) return;
    const ep = pendingRedownloadEp;
    pendingRedownloadEp = null;
    actionLoading = { ...actionLoading, [ep.episode_number]: true };
    try {
      await redownloadEpisode(animeId, ep.episode_number);
      toast.success(m.detail_toast_redownload({ number: ep.episode_number }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_redl_error());
    } finally {
      actionLoading = { ...actionLoading, [ep.episode_number]: false };
    }
  }

  async function handleBulkDownload() {
    const targets = selectedList.filter(ep => ep.is_aired && !ep.is_downloaded);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await downloadEpisode(animeId, ep.episode_number);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_dl_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  function handleBulkDelete() {
    confirmBulkOpen = true;
  }

  async function confirmBulkDelete() {
    const targets = selectedList.filter(ep => ep.is_downloaded);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await deleteEpisode(animeId, ep.episode_number);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_del_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  async function handleBulkRelease() {
    const targets = selectedList.filter(ep => ep.is_manually_managed || ep.is_blocked);
    bulkLoading = true;
    let success = 0, failed = 0;
    await Promise.all(targets.map(async ep => {
      try {
        await releaseEpisode(animeId, ep.episode_number);
        success++;
      } catch {
        failed++;
      }
    }));
    bulkLoading = false;
    if (success > 0) toast.success(m.detail_bulk_toast_rel_done({ success }));
    if (failed > 0) toast.error(m.detail_bulk_toast_partial({ failed }));
    selectedEpisodes = new Set();
    await loadData(animeId);
  }

  function handleReplace(ep: AnimeEpisodeInfo) {
    pendingReplaceEp = ep;
    replaceEpMagnet = "";
    replaceEpOpen = true;
  }

  async function confirmReplaceEp() {
    if (!pendingReplaceEp) return;
    if (!replaceEpMagnet.startsWith("magnet:")) {
      toast.error(m.detail_replace_invalid_magnet());
      return;
    }
    const ep = pendingReplaceEp;
    replaceLoading = true;
    try {
      await replaceEpisodeWithMagnet(animeId, ep.episode_number, replaceEpMagnet);
      toast.success(m.detail_replace_ep_done({ number: ep.episode_number }));
      replaceEpOpen = false;
      pendingReplaceEp = null;
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_replace_ep_error());
    } finally {
      replaceLoading = false;
    }
  }

  async function confirmReplaceAnime() {
    if (!replaceAnimeMagnet.startsWith("magnet:")) {
      toast.error(m.detail_replace_invalid_magnet());
      return;
    }
    replaceLoading = true;
    try {
      await replaceAnimeWithMagnet(animeId, replaceAnimeMagnet);
      toast.success(m.detail_replace_anime_done());
      replaceAnimeOpen = false;
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_replace_anime_error());
    } finally {
      replaceLoading = false;
    }
  }

  async function handleSaveSearchQuery() {
    searchQuerySaving = true;
    try {
      await updateAnimeSettings(animeId, { custom_search_query: customSearchQuery });
      toast.success(m.detail_search_query_saved());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_search_query_error());
    } finally {
      searchQuerySaving = false;
    }
  }

  $: loadData(animeId);

  // Adaptive polling for /torrents: 2s while this anime has an active (non-completed)
  // torrent, 15s otherwise. Re-scheduled (not a bare setInterval) so the delay can change
  // between ticks without ever running two timers at once.
  async function pollTorrents() {
    // allSettled pelo mesmo motivo do Status: uma falha do relatório não pode derrubar o poll
    // de torrents (nem o reagendamento dele).
    const [torrentsResult, reportResult] = await Promise.allSettled([
      getTorrents(),
      getLastCheck(),
    ]);

    // O finally continua sendo o que garante o reagendamento: allSettled nunca rejeita, mas
    // stallTracker.sync pode lançar, e sem o finally o poll pararia de vez em vez de se
    // recuperar no tick seguinte.
    try {
      if (torrentsResult.status === "fulfilled") {
        torrents = torrentsResult.value;
        lastPollAt = Date.now();
        stallTracker.sync(torrents, lastPollAt);
      } else {
        // Best-effort accessory data — no toast, keep the last known snapshot.
        console.error("Failed to load torrents:", torrentsResult.reason);
      }

      if (reportResult.status === "fulfilled") {
        const report = reportResult.value;
        animeIssues = [...report.problems, ...report.limits].filter((i) => i.anime_id === animeId);
      }
    } finally {
      scheduleNextTorrentPoll();
    }
  }

  function scheduleNextTorrentPoll() {
    if (torrentPollTimer) clearTimeout(torrentPollTimer);
    const map = indexTorrentsByEpisode(torrents, allEpisodes);
    const hasActiveTorrent = Array.from(map.values()).some((t) => !t.completed);
    torrentPollTimer = setTimeout(pollTorrents, hasActiveTorrent ? 2000 : 15000);
  }

  // svelte-spa-router reuses this component instance rather than remounting it when
  // navigating between two /status/:id routes (e.g. anime A -> anime B), so onDestroy never
  // runs on that transition and a plain "started once" flag would stay latched forever from
  // anime A. Re-arm the gate whenever animeId changes, and cancel the pending timer right
  // away rather than letting it fire on anime A's old cadence: without this, the page could
  // sit on anime A's 15s interval for up to ~15s after landing on anime B even though B has
  // an active download that should be polled every 2s.
  $: if (animeId !== torrentPollAnimeId) {
    torrentPollAnimeId = animeId;
    if (torrentPollTimer) {
      clearTimeout(torrentPollTimer);
      torrentPollTimer = null;
    }
    torrentPollStarted = false;
  }

  // Gate the very first poll (per anime) on `detail` actually belonging to the anime we're
  // now viewing — not just being non-null. Right after an animeId change (including the very
  // first mount), `$: loadData(animeId)` is still an in-flight promise; until it resolves,
  // `detail` either is null (first mount) or still holds the *previous* anime's episodes
  // (same-component switch, since svelte-spa-router reuses the instance). Firing on mere
  // non-null-ness would let this anime's first poll run against the wrong anime's episode
  // list — `indexTorrentsByEpisode` would join torrents against stale `episode_hash` values
  // and could pick the wrong cadence (reproduced by a component test that raced
  // getAnimeDetail() against getTorrents()). Comparing `detail.anime_id` to `animeId` ensures
  // the poll only ever starts once the episode list actually matches the anime being viewed,
  // so the very first cadence decision for a newly-selected anime is correct from the start.
  $: if (detail && detail.anime_id === animeId && !torrentPollStarted) {
    torrentPollStarted = true;
    pollTorrents();
  }

  onDestroy(() => {
    if (torrentPollTimer) clearTimeout(torrentPollTimer);
  });

  // Grid compartilhado pelo cabeçalho e pelas linhas (desktop), definido uma vez para que não
  // possam sair de alinhamento.
  //
  // A coluna de ações tem largura FIXA, não `auto`: cada linha é um grid independente, então
  // uma trilha `auto` mediria o conteúdo daquela linha só — e como as linhas têm ações
  // diferentes (principal + ⋯ / só ⋯ / nenhuma), o `1fr` sobrava diferente em cada uma e as
  // colunas ficavam desalinhadas entre si. Com todas as trilhas determinísticas, o `1fr`
  // resolve igual em toda linha. 200px comporta o rótulo mais longo ("Release episode" /
  // "Soltar episódio") mais o botão de 32px do menu.
  //
  // A tabela só existe a partir de `lg` (1024px): as trilhas fixas + gaps somam ~710px e em
  // `md` (768px) sobram ~628px depois do rail e do padding do main, o que jogava a página
  // inteira em rolagem horizontal. Mesmo critério da lista de animes (Status.svelte, LIST_GRID)
  // e das linhas de torrent (Downloads.svelte, ROW_GRID).
  async function confirmUntrack(): Promise<void> {
    if (!anime) return;
    untrackLoading = true;
    try {
      await removeStandaloneAnime(anime.anime_id, untrackDeleteFiles);
      toast.success(m.detail_untrack_done());
      window.location.hash = "#/status";
    } catch (error) {
      toast.error(error instanceof Error ? error.message : m.detail_untrack_btn());
    } finally {
      untrackLoading = false;
      untrackDeleteFiles = false;
    }
  }

  const EP_GRID =
    "grid grid-cols-[28px_52px_minmax(0,1fr)_190px_180px_200px] items-center gap-3";
</script>

<ConfirmDialog
  bind:open={confirmOpen}
  title={m.detail_confirm_title()}
  message={pendingDeleteEp ? m.detail_confirm_msg({ number: pendingDeleteEp.episode_number }) : ""}
  confirmLabel={m.detail_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmDelete}
/>

<ConfirmDialog
  bind:open={confirmRedownloadOpen}
  title={m.detail_redownload_confirm_title()}
  message={pendingRedownloadEp ? m.detail_redownload_confirm_msg({ number: pendingRedownloadEp.episode_number }) : ""}
  confirmLabel={m.detail_redownload_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmRedownload}
/>

<!-- scope="episode" (não "anime"): DELETE /torrents/{hash} só bloqueia os episódios que
     compartilham ESTE hash (RemoveTorrentWithEpisodes, daemon/episodes.go) — packs irmãos do
     mesmo anime continuam vivos. "anime" prometeria um alcance maior do que a operação cobre, e
     `standalone` fica de fora porque em scope="episode" o diálogo a ignora (checkbox de
     bloquear/rebaixar episódio, não de destrackear anime). -->
<TorrentDeleteDialog
  bind:open={packDeleteOpen}
  name={pendingPackLabel}
  scope="episode"
  on:confirm={(e) => confirmPackDelete(e.detail)}
  on:cancel={() => (packDeleteOpen = false)}
/>

<ConfirmDialog
  bind:open={confirmBulkOpen}
  title={m.detail_bulk_confirm_title()}
  message={m.detail_bulk_confirm_msg({ count: selectedList.filter(ep => ep.is_downloaded).length })}
  confirmLabel={m.detail_bulk_confirm_btn()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmBulkDelete}
/>

<!-- ConfirmDialog + um Checkbox no slot. O TorrentDeleteDialog não serve aqui: ele é
     específico de keepData/block, que são perguntas de torrent, não de anime. -->
<ConfirmDialog
  bind:open={untrackOpen}
  title={m.detail_untrack_title()}
  message={m.detail_untrack_msg()}
  confirmLabel={m.detail_untrack_confirm()}
  cancelLabel={m.common_cancel()}
  on:confirm={confirmUntrack}
>
  <Checkbox bind:checked={untrackDeleteFiles} label={m.detail_untrack_delete_files()} />
</ConfirmDialog>

<!-- Os dois diálogos de magnet agora usam o primitivo Modal (foco preso, Esc, clique fora),
     no lugar das duas overlays feitas à mão que não prendiam foco nem fechavam com Esc. -->
<Modal open={replaceEpOpen} labelledBy="replace-ep-title" on:close={() => (replaceEpOpen = false)}>
  <h3 id="replace-ep-title" class="text-card-title text-heading">
    {m.detail_replace_ep_title({ number: pendingReplaceEp?.episode_number ?? "" })}
  </h3>
  <input
    type="text"
    bind:value={replaceEpMagnet}
    placeholder={m.detail_replace_magnet_placeholder()}
    class="mt-4 w-full rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle focus:border-accent"
    on:keydown={(e) => { if (e.key === 'Enter') confirmReplaceEp(); }}
  />
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" on:click={() => (replaceEpOpen = false)}>{m.common_cancel()}</Button>
    <Button variant="solid" disabled={replaceLoading} on:click={confirmReplaceEp}>
      {replaceLoading ? "..." : m.detail_btn_replace()}
    </Button>
  </div>
</Modal>

<Modal open={replaceAnimeOpen} labelledBy="replace-anime-title" on:close={() => (replaceAnimeOpen = false)}>
  <h3 id="replace-anime-title" class="text-card-title text-heading">
    {m.detail_replace_anime_title()}
  </h3>
  <input
    type="text"
    bind:value={replaceAnimeMagnet}
    placeholder={m.detail_replace_magnet_placeholder()}
    class="mt-4 w-full rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle focus:border-accent"
    on:keydown={(e) => { if (e.key === 'Enter') confirmReplaceAnime(); }}
  />
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" on:click={() => (replaceAnimeOpen = false)}>{m.common_cancel()}</Button>
    <Button variant="solid" disabled={replaceLoading} on:click={confirmReplaceAnime}>
      {replaceLoading ? "..." : m.detail_btn_replace()}
    </Button>
  </div>
</Modal>

<div class="space-y-4.5">
  <!-- Cabeçalho -->
  <div>
    <nav class="flex items-center gap-1 text-caption text-subtle" aria-label={m.detail_back()}>
      <a href="#/status" class="hover:text-body">{$locale && m.nav_status()}</a>
      <ChevronRight size={13} strokeWidth={2} aria-hidden="true" />
      <span class="truncate text-body">{anime?.name ?? ($locale && m.detail_title_fallback())}</span>
    </nav>

    <div class="mt-3 flex flex-wrap items-start gap-4">
      <div class="h-[110px] w-[78px] shrink-0 overflow-hidden">
        <Cover src={detail?.cover_image} alt={anime?.name ?? ""} radiusClass="rounded-card" />
      </div>

      <!-- `min-w-[200px]`, não `min-w-0`: com `flex-wrap`, uma coluna que pode encolher até
           zero nunca empurra o botão para a linha de baixo — no mobile o título espremia em
           três linhas e o "Replace Anime with Magnet" ficava por cima dele. Com um piso de
           largura, o botão quebra para a própria linha quando não cabe. -->
      <div class="min-w-[200px] flex-1">
        <h1 class="text-[24px] font-bold leading-tight text-heading">
          {#if anime && detail?.anime_id}
            <a
              href="https://anilist.co/anime/{detail.anime_id}"
              target="_blank"
              rel="noopener noreferrer"
              class="hover:underline"
            >
              {anime.name}
            </a>
          {:else}
            {anime ? anime.name : ($locale && m.detail_title_fallback())}
          {/if}
        </h1>

        {#if animeChip}
          <div class="mt-2">
            <Chip variant={animeChip.variant} dimmed={animeChip.dimmed}>{$locale && chipLabel(animeChip)}</Chip>
          </div>
        {/if}

        {#if anime}
          <p class="mt-2 font-mono text-caption text-subtle">
            {$locale && m.detail_counts({
              downloaded: anime.episodes_downloaded,
              total: anime.total_episodes || "?",
              watched: anime.episodes_watched,
            })}
            {#if detail?.status}
              · {m.detail_anilist_status({ status: detail.status })}
            {/if}
            {#if nextAiringWhen}
              · {m.next_airing({ when: nextAiringWhen })}
            {/if}
          </p>

          <!-- Avulso não tem progresso na AniList: é aqui que ele mora, e é o progresso que move
               o rodízio de packs (sem ele, depois do primeiro pack o anime para para sempre). -->
          {#if anime.is_standalone}
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <label for="standalone-progress" class="text-copy text-body">
                {$locale && m.detail_progress_label()}
              </label>
              <input
                id="standalone-progress"
                type="number"
                min="0"
                bind:value={progressInput}
                class="w-20 rounded-field border border-default bg-control px-2 py-1 text-copy text-heading outline-none focus:border-accent"
                on:keydown={(e) => { if (e.key === 'Enter') saveProgress(progressInput); }}
              />
              <Button variant="ghost" disabled={progressSaving} on:click={() => saveProgress(progressInput)}>
                {progressSaving ? "..." : ($locale && m.common_save())}
              </Button>
            </div>
            <p class="mt-1 text-caption text-subtle">{$locale && m.detail_progress_hint()}</p>
          {/if}
        {/if}
      </div>

      <div class="flex flex-wrap gap-2">
        {#if anime?.is_standalone}
          <Button variant="warn" disabled={untrackLoading} on:click={() => { untrackDeleteFiles = false; untrackOpen = true; }}>
            {$locale && m.detail_untrack_btn()}
          </Button>
        {/if}
        <Button variant="ghost" on:click={() => { replaceAnimeMagnet = ""; replaceAnimeOpen = true; }}>
          {$locale && m.detail_replace_btn_anime()}
        </Button>
      </div>
    </div>
  </div>

  <!-- Busca customizada do anime — recolhível (spec §9.2) -->
  {#if detail && !anime?.is_standalone}
    <section class="rounded-card border border-default bg-card">
      <button
        type="button"
        class="flex w-full items-center gap-2 px-4 py-3 text-left text-copy text-body transition-colors hover:bg-control"
        aria-expanded={searchQueryOpen}
        on:click={() => (searchQueryOpen = !searchQueryOpen)}
      >
        <ChevronDown
          size={16}
          strokeWidth={2}
          class="shrink-0 transition-transform {searchQueryOpen ? '' : '-rotate-90'}"
        />
        {$locale && m.detail_search_query_label()}
        {#if customSearchQuery}
          <span class="truncate font-mono text-caption text-subtle">{customSearchQuery}</span>
        {/if}
      </button>

      {#if searchQueryOpen}
        <div class="border-t border-divider p-4">
          <label for="custom-search-query" class="mb-1.5 block text-copy text-body">
            {$locale && m.detail_search_query_label()}
          </label>
          <div class="flex flex-wrap items-center gap-2">
            <input
              id="custom-search-query"
              type="text"
              bind:value={customSearchQuery}
              placeholder={m.detail_search_query_placeholder()}
              class="min-w-0 flex-1 rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle focus:border-accent"
              on:keydown={(e) => { if (e.key === 'Enter') handleSaveSearchQuery(); }}
            />
            <Button variant="ghost" disabled={searchQuerySaving} on:click={handleSaveSearchQuery}>
              {searchQuerySaving ? "..." : ($locale && m.common_save())}
            </Button>
          </div>
          <p class="mt-1.5 text-caption text-subtle">{$locale && m.detail_search_query_placeholder()}</p>
        </div>
      {/if}
    </section>
  {/if}

  {#if loading}
    <Loading message={m.detail_loading()} />
  {:else if !detail || detail.episodes.length === 0}
    <div class="rounded-card border border-default bg-card p-6">
      <p class="text-copy text-subtle">{$locale && m.detail_empty()}</p>
    </div>
  {:else}
    <section class="rounded-card border border-default bg-card">
      <!-- Barra de seleção múltipla. Só as três ações em lote que EXISTEM hoje: Baixar,
           Soltar, Excluir. "Rebaixar" em lote não é adicionado — seria funcionalidade nova
           (spec §9.2), não redesenho. -->
      {#if selectedEpisodes.size > 0}
        <div class="flex flex-wrap items-center gap-2 border-b border-accent bg-accent-tint/[.09] px-4 py-2.5">
          <span class="mr-auto text-copy text-accent">
            {$locale && m.detail_bulk_selected({ count: selectedEpisodes.size })}
          </span>
          {#if canBulkDownload}
            <Button variant="solid" disabled={bulkLoading} on:click={handleBulkDownload}>
              {$locale && m.detail_bulk_download()}
            </Button>
          {/if}
          {#if canBulkRelease}
            <Button variant="ghost" disabled={bulkLoading} on:click={handleBulkRelease}>
              {$locale && m.detail_bulk_release()}
            </Button>
          {/if}
          {#if canBulkDelete}
            <Button variant="warn" disabled={bulkLoading} on:click={handleBulkDelete}>
              {$locale && m.detail_bulk_delete()}
            </Button>
          {/if}
          <Button variant="ghost" on:click={() => (selectedEpisodes = new Set())}>
            {$locale && m.detail_bulk_deselect_all()}
          </Button>
        </div>
      {/if}

      {#if animeIssues.length > 0}
        <div
          data-testid="anime-last-check"
          role="status"
          class="mb-3 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
        >
          <p>{$locale && m.detail_lastcheck_notice()}</p>
          <ul class="mt-1 space-y-1 text-caption">
            {#each animeIssues as issue (issue.code)}
              <li>
                {issueMessage(issue)}{#if batchNote(issue)} · {batchNote(issue)}{/if}
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <!-- Desktop -->
      <div class="hidden lg:block">
        <div class="{EP_GRID} border-b border-divider px-4 py-2.5">
          <Checkbox
            checked={allSelected}
            indeterminate={someSelected}
            label={$locale ? m.detail_select_all() : ""}
            labelHidden
            on:change={toggleSelectAll}
          />
          <span class="font-mono text-mono-label uppercase text-subtle">#</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_episode()}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_state()}</span>
          <!-- "Detalhes", não "Data de download": esta coluna mostra o dado mais informativo
               para o estado da linha (progresso em voo / data de download / falta X para
               estrear), então um cabeçalho que nomeasse só um dos três estaria errado nos
               outros dois. -->
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_info()}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_actions()}</span>
        </div>

        {#each rows || [] as row (row.key)}
          {@const isLoading = !!actionLoading[row.ep.episode_number]}
          {@const isSelected = row.kind === "episode" && selectedEpisodes.has(row.ep.episode_number)}
          <div
            data-episode-row
            class="{EP_GRID} border-b border-divider px-4 py-3 last:border-b-0 {isSelected ? 'bg-accent-tint/[.06]' : ''}"
          >
            {#if row.kind === "episode"}
              <Checkbox
                checked={isSelected}
                label={$locale ? m.detail_select_episode({ number: row.ep.episode_number }) : ""}
                labelHidden
                on:change={() => toggleEpisode(row.ep.episode_number)}
              />
            {:else}
              <span></span>
            {/if}

            <span class="font-mono text-[15px] font-bold text-heading">{row.label}</span>

            <div class="min-w-0">
              <p class="truncate text-copy text-heading" title={row.title}>{row.title}</p>
              {#if row.notes}
                <p class="truncate text-caption text-subtle">{row.notes}</p>
              {/if}
            </div>

            <div class="min-w-0">
              <Chip variant={row.chip.variant}>{row.chip.label}</Chip>
              <!-- Barra fina só enquanto o torrent está em voo. A condição é APENAS
                   `!torrent.completed` — ver o comentário grande no topo do script sobre
                   `episode_hash != "" ⟺ is_downloaded`. -->
              {#if row.inFlight && row.torrent}
                <div class="mt-1.5">
                  <ProgressBar
                    value={row.torrent.progress}
                    thickness={4}
                    label={m.detail_torrent_progress_aria()}
                  />
                </div>
              {/if}
            </div>

            <span class="truncate font-mono text-[12px] text-subtle" title={row.meta}>{row.meta}</span>

            <!-- Coluna de ações: a principal SEMPRE na mesma posição, com texto; o resto no ⋯,
                 também rotulado. Desktop e mobile leem esta mesma definição. -->
            <div class="flex items-center justify-end gap-1.5">
              {#if row.principal}
                <Button
                  variant={row.principal.variant}
                  disabled={isLoading}
                  on:click={() => row.principal && runAction(row.principal.id, row.ep)}
                >
                  {actionLabel(row.principal)}
                </Button>
              {/if}
              {#if row.menu.length > 0}
                <ActionMenu
                  items={row.menu}
                  triggerLabel={row.kind === "batch" ? m.detail_batch_actions_menu() : m.detail_actions_menu({ number: row.ep.episode_number })}
                  on:select={(e) => row.kind === "batch" ? openPackDelete(row) : runAction(e.detail as EpisodeActionId, row.ep)}
                />
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- Mobile: mesma definição de linha, empilhada -->
      <div class="divide-y divide-divider lg:hidden">
        {#each rows || [] as row (row.key)}
          {@const isLoading = !!actionLoading[row.ep.episode_number]}
          {@const isSelected = row.kind === "episode" && selectedEpisodes.has(row.ep.episode_number)}
          <div data-episode-row class="p-4 {isSelected ? 'bg-accent-tint/[.06]' : ''}">
            <div class="flex items-start gap-3">
              {#if row.kind === "episode"}
                <Checkbox
                  checked={isSelected}
                  label={$locale ? m.detail_select_episode({ number: row.ep.episode_number }) : ""}
                  labelHidden
                  on:change={() => toggleEpisode(row.ep.episode_number)}
                />
              {:else}
                <span></span>
              {/if}
              <span class="font-mono text-[15px] font-bold text-heading">{row.label}</span>
              <div class="min-w-0 flex-1">
                <p class="truncate text-copy text-heading">{row.title}</p>
                {#if row.notes}
                  <p class="truncate text-caption text-subtle">{row.notes}</p>
                {/if}
              </div>
            </div>

            <div class="mt-2.5">
              <Chip variant={row.chip.variant}>{row.chip.label}</Chip>
              {#if row.inFlight && row.torrent}
                <div class="mt-1.5">
                  <ProgressBar
                    value={row.torrent.progress}
                    thickness={4}
                    label={m.detail_torrent_progress_aria()}
                  />
                </div>
              {/if}
              <p class="mt-1.5 font-mono text-[12px] text-subtle">{row.meta}</p>
            </div>

            <div class="mt-3 flex items-center gap-1.5">
              {#if row.principal}
                <Button
                  variant={row.principal.variant}
                  disabled={isLoading}
                  on:click={() => row.principal && runAction(row.principal.id, row.ep)}
                >
                  {actionLabel(row.principal)}
                </Button>
              {/if}
              {#if row.menu.length > 0}
                <ActionMenu
                  items={row.menu}
                  triggerLabel={row.kind === "batch" ? m.detail_batch_actions_menu() : m.detail_actions_menu({ number: row.ep.episode_number })}
                  on:select={(e) => row.kind === "batch" ? openPackDelete(row) : runAction(e.detail as EpisodeActionId, row.ep)}
                />
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/if}
</div>
