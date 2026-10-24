<script lang="ts">
  // AnimeDetail — spec §9.2 (Phase 4). The highest-gain screen of the redesign: the previous
  // version was 971 lines largely because the action list was written TWICE (desktop table +
  // mobile cards), with the same five icon-only buttons whose `title` was the only textual
  // cue and which swapped position between rows as they appeared and disappeared.
  //
  // Now there is ONE action definition — `episodeActions()` (lib/domain/), pure data — that
  // desktop and mobile render through the same `ActionMenu`. The principal action has text
  // and stays in the same column; the rest lives in the `⋯`, also labelled.
  import { onDestroy } from "svelte";
  import { push } from "svelte-spa-router";
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
  import TorrentFiles from "../components/TorrentFiles.svelte";
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

  // Stop tracking (standalone only). Lives in the AnimeDetail header because that's where
  // the user already is after clicking the anime in the Status list, and the header is
  // already the place for anime actions — putting this on the Status row would cost an
  // ActionMenu column the list doesn't have today.
  let untrackOpen = false;
  let untrackDeleteFiles = false;
  let untrackLoading = false;

  // Per-episode magnet replace. Whole-title replace is the picker at
  // `#/status/:id/torrents` — there is no magnet modal for the header button.
  let replaceEpOpen = false;
  let pendingReplaceEp: AnimeEpisodeInfo | null = null;
  let replaceEpMagnet = "";
  let replaceLoading = false;

  // Custom search query state. The handoff forgot this field; spec §9.2 asks for it to come
  // back in a collapsible block right under the header — collapsed by default because it is
  // a fine-tuning, not what you come to this screen to do.

  // Manual progress of the standalone. Prefill from `anime.episodes_watched`, which already
  // carries the saved value (the backend injects AnimeSettings.progress into the synthetic MediaList).
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
  // Only this anime's entries. Same call as Status, filtered by anime_id: one endpoint with
  // two consumers instead of the same information spread across three shapes that would need
  // to agree with each other.
  let animeIssues: Issue[] = [];
  // Only a problem marks an episode: a limit has no "affected episodes" (the daemon stopped
  // considering episodes once it hit the count).
  $: issueByEpisode = new Map<number, Issue>(
    animeIssues.flatMap((i) => (i.episodes ?? []).map((n) => [n, i] as [number, Issue])),
  );
  let torrentPollTimer: ReturnType<typeof setTimeout> | null = null;
  let torrentPollStarted = false;
  let torrentPollAnimeId: number | null = null;
  // Stamp of the last successful poll. `deriveAnimeChip` only uses `now` for the "no seeds"
  // threshold; updating on a clock tick would make the whole screen recalculate every second
  // without any data having changed.
  let lastPollAt = Date.now();

  // Countdown to the next episode. Anchored on `lastPollAt` for the same reason as the
  // comment above — the text is in hours/days, nothing here justifies a one-second tick.
  $: nextAiringWhen = $locale ? nextAiringIn(anime?.next_airing_at, lastPollAt, $locale) : "";

  $: allEpisodes = detail?.episodes ?? [];
  $: torrentsByEpisode = indexTorrentsByEpisode(torrents, allEpisodes);

  // CAUTION when touching the progress inline below: an episode only gets `episode_hash`
  // when the daemon writes the saved record — and it writes at the moment the torrent is
  // *added*, not when it finishes (daemon/episodes.go, daemon/manual_download.go). Since
  // endpoint_anime_episodes.go fills `is_downloaded` and `episode_hash` in the same `if`,
  // `episode_hash != "" ⟺ is_downloaded` holds. So the inline condition cannot require
  // `!ep.is_downloaded`: that combination never occurs in practice and the bar never showed.
  // What decides is only that the torrent is still in flight (`!torrent.completed`).
  $: allSelected = allEpisodes.length > 0 && allEpisodes.every(ep => selectedEpisodes.has(ep.episode_number));
  $: someSelected = selectedEpisodes.size > 0 && !allSelected;

  $: selectedList = allEpisodes.filter(ep => selectedEpisodes.has(ep.episode_number));
  $: canBulkDownload = selectedList.some(ep => ep.is_aired && !ep.is_downloaded);
  $: canBulkDelete = selectedList.some(ep => ep.is_downloaded);
  $: canBulkRelease = selectedList.some(ep => ep.is_manually_managed || ep.is_blocked);

  // Anime chip in the header — same cascade as the Status list, same component.
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
   * State of ONE episode, for the `Chip` of the "State" column. An in-flight torrent wins
   * everything — it's the most volatile information and the only one the user is actively
   * watching — and the label comes from `statusLabel`, which already translates rain's raw
   * slugs (spec §7: states never appear as identifiers).
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

  /** Secondary note under the title: episode flags, as a phrase, not a mute icon. */
  function episodeNotes(ep: AnimeEpisodeInfo, issue: Issue | undefined): string {
    const notes: string[] = [];
    if (ep.is_watched) notes.push(m.detail_badge_watched());
    if (ep.is_manually_managed) notes.push(m.detail_flag_no_delete_short());
    if (ep.is_blocked) notes.push(m.detail_flag_no_download_short());
    // The report mark goes on the same line as the other notes: it's a short phrase, not a
    // live state — deriveAnimeChip stays out of this on purpose (see decisions.md).
    if (issue) notes.push(m.detail_lastcheck_episode_note());
    return notes.join(" · ");
  }

  /** Row metadata: the most informative datum for the episode's current state. */
  function episodeMeta(ep: AnimeEpisodeInfo, torrent: TorrentInfo | undefined): string {
    if (torrent && !torrent.completed) {
      return `${formatPercent(torrent.progress, fmtLocale)} · ↓ ${formatSpeed(torrent.download_speed, fmtLocale)} · ${formatEta(torrent.eta_seconds, fmtLocale)}`;
    }
    if (ep.is_downloaded) return formatDate(ep.download_date);
    if (!ep.is_aired) return formatTimeUntilAiring(ep.time_until_airing, ep.is_aired);
    return "—";
  }

  // `episodeActions` returns `labelKey`, an identifier — never text. This is its only
  // translation on the screen, shared by the principal action and the `⋯` items.
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
   * Single dispatch point for the five actions. `delete` and `redownload` still go through
   * the confirmation dialogs that already existed — spec §7 is explicit: deletion is never
   * a single click, and `episodeActions` marks both as `destructive` precisely to signal that.
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

  // One pass builds everything each row needs, so desktop and mobile read the SAME
  // structure — that's what keeps the two copies from diverging again.
  //
  // A pack is ONE row. The detection rule is the same the backend already uses in
  // buildTorrentResponse: more than one episode on the same info hash is a batch, whatever
  // the flag says. Deleting an episode from inside a pack frees no bytes (the library
  // hardlink and the seed copy are the same inode) and "Redownload" would look for a single
  // episode, which doesn't exist in a long series — so the group row only offers what is
  // true for a pack.
  //
  // Not even "Replace": that button calls replaceAnimeWithMagnet, which deletes ALL of the
  // anime's torrents (endpoint_episode_actions.go), not just the clicked pack's — with
  // successive packs (001-100 + 101-200), "Replace" on the first would silently destroy the
  // second. The same argument as the comment above about "Redownload" applies here: only
  // "Delete" remains, which already goes through DELETE /torrents/{hash} and operates on
  // exactly the right unit.
  //
  // ponytail: buildRows is O(n²) (the .filter() per pack inside the main loop) — irrelevant
  // for the real size of an anime, but if this screen one day lists thousands of episodes
  // across dozens of packs, swapping in a Map<hash, AnimeEpisodeInfo[]> built in one pass
  // would fix it.
  function buildRows(episodes: AnimeEpisodeInfo[], byEpisode: Map<number, TorrentInfo>) {
    const counts = new Map<string, number>();
    for (const ep of episodes) {
      if (ep.episode_hash) counts.set(ep.episode_hash, (counts.get(ep.episode_hash) ?? 0) + 1);
    }

    // One pack per hash, with the range it actually covers. The range comes from the record
    // (read from the torrent name at download), not from min/max of the saved episodes: already
    // watched ones never got a record, so min/max shrinks the pack — a downloaded 01-11 with
    // 5 watched became "6–11". With no saved range (old record) min/max remains the best guess.
    //
    // The start is clamped to 1: a season pack downloaded under a later cour is stored with
    // start ≤ 0 in this entry's local numbering (that's what coverage ownership needs), and
    // "-10–12" says nothing on screen — what matters here is this anime's slice.
    const packs = new Map<string, { first: number; last: number }>();
    for (const ep of episodes) {
      const hash = ep.episode_hash;
      if (!hash || (counts.get(hash) ?? 0) < 2 || packs.has(hash)) continue;
      const numbers = episodes.filter((e) => e.episode_hash === hash).map((e) => e.episode_number);
      packs.set(hash, {
        first: Math.max(1, ep.batch_start || Math.min(...numbers)),
        last: ep.batch_end || Math.max(...numbers),
      });
    }

    // The pack swallows episodes inside the range that don't have their own torrent —
    // typically the already-watched ones, which never became a record. They ARE in the pack:
    // the torrent brings the files and Organize hardlinks the whole directory into the
    // library, so a "Download" row for them offered a second torrent of what is already
    // coming. Unreleased is not swallowed — the pack on disk cannot contain what hasn't aired.
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
        // Always the localized title. There is no episode title anywhere: AniList doesn't
        // expose it per episode, and the saved record's `episode_name` is the download log
        // label ("Anime - Episode 7"), which the API no longer returns.
        title: m.detail_ep_title({ number: ep.episode_number }),
        label: String(ep.episode_number),
      });
    }

    return out;
  }

  $: rows = $locale && issueByEpisode && buildRows(allEpisodes, torrentsByEpisode);

  // Group-row action: only "Delete", via DELETE /torrents/{hash}, which already removes the
  // torrent and ALL of the group's records as a unit. Neither "Redownload" (would be an alias
  // of Delete, since the loop re-searches on the next cycle as soon as space comes back) nor
  // "Replace" (would silently destroy sibling packs — see the long comment in buildRows)
  // make sense here, so there is no branching by id: the only pack-menu item is already
  // "delete".
  //
  // ponytail: this action stays inline here, not in lib/domain/episodeActions.ts, because
  // today there is only ONE pack row on ONE screen, and it is a fixed constant (not a state
  // classification like episodeActions.ts's classify()). If a second screen needs the same
  // pack decision, then extracting an exported packActionSet() from episodeActions.ts is
  // worth it.
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

  $: loadData(animeId);

  // Adaptive polling for /torrents: 2s while this anime has an active (non-completed)
  // torrent, 15s otherwise. Re-scheduled (not a bare setInterval) so the delay can change
  // between ticks without ever running two timers at once.
  async function pollTorrents() {
    // allSettled for the same reason as Status: a report failure must not take down the
    // torrent poll (nor its reschedule).
    const [torrentsResult, reportResult] = await Promise.allSettled([
      getTorrents(),
      getLastCheck(),
    ]);

    // The finally is still what guarantees reschedule: allSettled never rejects, but
    // stallTracker.sync can throw, and without the finally the poll would stop for good
    // instead of recovering on the next tick.
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
      // Only OPEN panels refetch the file list, and on the same tick as this poll.
      filesTick += 1;
      scheduleNextTorrentPoll();
    }
  }

  // File panel of the pack row. It is the only row WITH A GAP: an episode with its own
  // torrent already shows individual progress (episodeMeta), and episodes inside a pack are
  // swallowed into one row on purpose — there is no per-episode row there to receive a bar
  // without expanding. Local, ephemeral state, outside the URL.
  let openPackFiles = new Set<string>();
  let filesTick = 0;

  function togglePackFiles(hash: string) {
    const next = new Set(openPackFiles);
    next.has(hash) ? next.delete(hash) : next.add(hash);
    openPackFiles = next;
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

  // Grid shared by the header and the rows (desktop), defined once so they can't fall out
  // of alignment.
  //
  // The actions column has a FIXED width, not `auto`: each row is an independent grid, so
  // an `auto` track would measure that row's content only — and since rows have different
  // actions (principal + ⋯ / only ⋯ / none), the leftover `1fr` differed on each one and
  // the columns misaligned with each other. With every track deterministic, `1fr` resolves
  // the same on every row. 200px fits the longest label ("Release episode" /
  // "Soltar episódio") plus the 32px menu button.
  //
  // The table only exists from `lg` (1024px): the fixed tracks + gaps add up to ~710px and
  // at `md` (768px) ~628px remain after the rail and the main padding, which threw the
  // whole page into horizontal scroll. Same criterion as the anime list (Status.svelte,
  // LIST_GRID) and the torrent rows (Downloads.svelte, ROW_GRID).
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
    "grid grid-cols-[28px_minmax(0,1fr)_190px_180px_200px] items-center gap-3";
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

<!-- scope="episode" (not "anime"): DELETE /torrents/{hash} only blocks the episodes that
     share THIS hash (RemoveTorrentWithEpisodes, daemon/episodes.go) — sibling packs of the
     same anime stay alive. "anime" would promise a wider reach than the operation covers, and
     `standalone` is left out because at scope="episode" the dialog ignores it (checkbox to
     block/redownload an episode, not to untrack an anime). -->
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

<!-- ConfirmDialog + a Checkbox in the slot. TorrentDeleteDialog doesn't belong here: it is
     specific to keepData/block, which are torrent questions, not anime ones. -->
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

<!-- Per-episode magnet dialog. Whole-title replace navigates to the picker. -->
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

<div class="space-y-4.5">
  <!-- Header -->
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

      <!-- `min-w-[200px]`, not `min-w-0`: with `flex-wrap`, a column that can shrink to zero
           never pushes the button onto the next line — on mobile the title squeezed into
           three lines and "Replace Anime with Magnet" sat on top of it. With a width floor,
           the button wraps onto its own line when it doesn't fit. -->
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

          <!-- A standalone has no AniList progress: this is where it lives, and it's the
               progress that moves the pack rotation (without it, after the first pack the
               anime stops forever). -->
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
        <Button variant="ghost" on:click={() => push("/status/" + animeId + "/torrents")}>
          {$locale && m.detail_replace_btn_anime()}
        </Button>
      </div>
    </div>
  </div>

  {#if loading}
    <Loading message={m.detail_loading()} />
  {:else if !detail || detail.episodes.length === 0}
    <div class="rounded-card border border-default bg-card p-6">
      <p class="text-copy text-subtle">{$locale && m.detail_empty()}</p>
    </div>
  {:else}
    <section class="rounded-card border border-default bg-card">
      <!-- Multi-select bar. Only the three bulk actions that EXIST today: Download, Release,
           Delete. Bulk "Redownload" is not added — that would be new functionality
           (spec §9.2), not a redesign. -->
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
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_episode()}</span>
          <span class="font-mono text-mono-label uppercase text-subtle">{$locale && m.detail_col_state()}</span>
          <!-- "Details", not "Download date": this column shows the most informative datum
               for the row's state (in-flight progress / download date / X until premiere),
               so a header that named only one of the three would be wrong on the other two. -->
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

            <div class="min-w-0">
              {#if row.kind === "batch" && row.hash}
                <!-- Only the pack row gets an arrow: the panel is the first time the screen
                     shows what the pack REALLY contains (the displayed range comes from the
                     torrent NAME). It lives on the title because there is no "#" column
                     anymore: the number and the title said the same thing in two typefaces. -->
                <button
                  type="button"
                  class="flex w-full items-center gap-1.5 text-left"
                  aria-expanded={openPackFiles.has(row.hash)}
                  aria-label={$locale ? m.files_toggle({ name: row.title }) : ""}
                  on:click={() => row.hash && togglePackFiles(row.hash)}
                >
                  <svelte:component
                    this={openPackFiles.has(row.hash) ? ChevronDown : ChevronRight}
                    size={14}
                    strokeWidth={2}
                    class="shrink-0 text-subtle"
                  />
                  <span class="truncate text-copy text-heading">{row.title}</span>
                </button>
              {:else}
                <p class="truncate text-copy text-heading" title={row.title}>{row.title}</p>
              {/if}
              {#if row.notes}
                <p class="truncate text-caption text-subtle">{row.notes}</p>
              {/if}
            </div>

            <div class="min-w-0">
              <Chip variant={row.chip.variant}>{row.chip.label}</Chip>
              <!-- Thin bar only while the torrent is in flight. The condition is ONLY
                   `!torrent.completed` — see the long comment at the top of the script about
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

            <!-- Actions column: the principal ALWAYS in the same position, with text; the rest
                 in the ⋯, also labelled. Desktop and mobile read this same definition. -->
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
          {#if row.kind === "batch" && row.hash && openPackFiles.has(row.hash)}
            <TorrentFiles
              hash={row.hash}
              mode="episodes"
              tick={filesTick}
              gridClass={EP_GRID}
              chip={row.chip}
            />
          {/if}
        {/each}
      </div>

      <!-- Mobile: same row definition, stacked -->
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
              {#if row.kind === "batch" && row.hash}
                <!-- Same decision as desktop: the arrow lives on the title, not in a "#"
                     column that would repeat the number the title already says. -->
                <button
                  type="button"
                  class="flex min-w-0 flex-1 items-center gap-1.5 text-left"
                  aria-expanded={openPackFiles.has(row.hash)}
                  aria-label={$locale ? m.files_toggle({ name: row.title }) : ""}
                  on:click={() => row.hash && togglePackFiles(row.hash)}
                >
                  <svelte:component
                    this={openPackFiles.has(row.hash) ? ChevronDown : ChevronRight}
                    size={14}
                    strokeWidth={2}
                    class="shrink-0 text-subtle"
                  />
                  <span class="truncate text-copy text-heading">{row.title}</span>
                </button>
              {:else}
                <div class="min-w-0 flex-1">
                  <p class="truncate text-copy text-heading">{row.title}</p>
                  {#if row.notes}
                    <p class="truncate text-caption text-subtle">{row.notes}</p>
                  {/if}
                </div>
              {/if}
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
          {#if row.kind === "batch" && row.hash && openPackFiles.has(row.hash)}
            <TorrentFiles hash={row.hash} mode="episodes" tick={filesTick} chip={row.chip} />
          {/if}
        {/each}
      </div>
    </section>
  {/if}
</div>
