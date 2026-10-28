<script lang="ts">
  // Status — spec §9.1 (Phase 3). It's still daemon AND anime list (decision D4): the handoff's
  // separate "Library" screen doesn't exist. New layout from top to bottom: header with the daemon
  // pill, speed hero card, right column (library + disk/next check) and the anime list.
  //
  // The artboard 1b "Queue and problems" section was DROPPED on purpose (spec §9.1): it's redundant
  // with the Downloads screen, which exists precisely for that.
  import { onMount, onDestroy } from "svelte";
  import { querystring, replace } from "svelte-spa-router";
  import { ChevronDown, ChevronRight, Eye, Search, X } from "@lucide/svelte";
  import {
    getStatus,
    getAnimes,
    getConfig,
    triggerCheck,
    startDaemon,
    stopDaemon,
    getTorrents,
    getLastCheck,
    type CheckReport,
    type StatusResponse,
    type AnimeInfo,
    type TorrentInfo,
  } from "../lib/api/client.js";
  import { WebSocketClient } from "../lib/websocket/client.js";
  import Loading from "../components/Loading.svelte";
  import ActionMenu from "../components/ui/ActionMenu.svelte";
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
  import {
    filterAnimes,
    sortAnimes,
    computeNextCheckIn,
    breakdown,
    encodeStatusView,
    decodeStatusView,
    type SortKey,
    type SortDir,
    type AnimeTypeFilter,
    type StatusViewState,
  } from "../lib/utils/status.js";
  import { totalSpeeds } from "../lib/utils/torrents.js";
  import { deriveAnimeChip, type AnimeChipState } from "../lib/domain/animeState.js";
  import { formatRelative, nextAiringIn } from "../lib/utils/relativeTime.js";
  import { issueMessage, batchNote } from "../lib/domain/checkIssue.js";
  import { passErrorMessage } from "../lib/domain/passError.js";
  import { onboardingSteps, allDone } from "../lib/domain/onboarding.js";
  import {
    ONBOARDING_STEP_IDS,
    onboardingDismissed,
    onboardingDone,
  } from "../lib/stores/onboarding.js";
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

  // Torrents poll interval. Exposed as a constant because the "polling Ns" note shown to the user
  // (spec §8: the UI states that progress comes from HTTP polling, not from the WebSocket) is
  // derived from here — never a number written by hand in the markup that could drift from the
  // timer.
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
    colNextAiring: m.status_col_next_airing(),
    colLastDownload: m.status_col_last_download(),
    emptyTitle: m.status_empty_title(),
    emptyDesc: m.status_empty_desc(),
    goToConfig: m.status_go_to_config(),
    filterUnwatched: m.status_filter_unwatched(),
    filterAll: m.status_filter_all(),
    filterAiring: m.status_filter_airing(),
    filterFinished: m.status_filter_finished(),
    filterStandalone: m.status_filter_standalone(),
    filterBlacklisted: m.status_filter_blacklisted(),
    filterType: m.status_filter_type(),
    clearFilters: m.status_clear_filters(),
    loading: m.status_loading(),
    heroSpeedLabel: m.status_hero_speed_label(),
    heroNoDownloads: m.status_hero_no_downloads(),
    activeDownloads: m.status_active_downloads_label(),
    pollingNote: m.status_polling_note({ seconds: TORRENTS_POLL_MS / 1000 }),
    staleNote: m.status_stale_note(),
    sortAsc: m.status_sort_asc(),
    sortDesc: m.status_sort_desc(),
    onboardingTitle: m.onboarding_title(),
    onboardingDismiss: m.onboarding_dismiss(),
    onboardingOr: m.onboarding_or(),
    onboardingActionConfigure: m.onboarding_action_configure(),
    onboardingActionConnectAnilist: m.onboarding_action_connect_anilist(),
    onboardingActionAddAnime: m.onboarding_action_add_anime(),
    onboardingActionCheckNow: m.onboarding_action_check_now(),
    reportProblems: (count: number) => m.lastcheck_section_problems({ count }),
    reportLimits: (count: number) => m.lastcheck_section_limits({ count }),
  };

  let status: StatusResponse | null = null;
  let animes: AnimeInfo[] = [];
  let torrents: TorrentInfo[] = [];
  // Report of the last pass. Fetched together with the torrents poll this screen already keeps —
  // the same cadence, and the cost of one more request per tick.
  let lastCheck: CheckReport | null = null;
  let checkInterval = 0;
  // With no library configured there's nowhere to download to: the add-anime button starts disabled
  // instead of letting the user find out only at the POST's 409. During load it stays enabled so it
  // doesn't flash disabled before `getConfig()` answers. The raw path and the usernames, beyond the
  // boolean: `onboardingSteps` takes the config, not an already-cooked boolean. No new request —
  // they all come from the same `getConfig()`.
  let completedPath = "";
  let anilistUsernames: string[] = [];
  let malUsernames: string[] = [];
  let loading = true;
  let actionLoading = false;
  let search = "";
  let filterUnwatched = false;
  let typeFilter: AnimeTypeFilter = "all";
  let viewReady = false;
  let lastWrittenQuery = "";
  let now = Date.now();
  // spec §8: progress doesn't come from the WebSocket, it comes from the HTTP poll. When a poll
  // fails the UI has to SAY so instead of keeping the last number on screen as if it were live.
  let torrentsStale = false;

  let sortKey: SortKey = "last_download_date";
  let sortDir: SortDir = "desc";

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  $: filteredAnimes = filterAnimes(animes, search, { unwatched: filterUnwatched, type: typeFilter });
  $: sortedAnimes = sortAnimes(filteredAnimes, sortKey, sortDir);

  $: typeCounts = {
    all: filterAnimes(animes, search, { unwatched: filterUnwatched, type: "all" }).length,
    airing: filterAnimes(animes, search, { unwatched: filterUnwatched, type: "airing" }).length,
    finished: filterAnimes(animes, search, { unwatched: filterUnwatched, type: "finished" }).length,
    standalone: filterAnimes(animes, search, { unwatched: filterUnwatched, type: "standalone" }).length,
    blacklisted: filterAnimes(animes, search, { unwatched: filterUnwatched, type: "blacklisted" }).length,
  };
  $: unwatchedCount = filterAnimes(animes, search, { unwatched: true, type: typeFilter }).length;
  $: filtersActive = search !== "" || filterUnwatched || typeFilter !== "all";

  $: typeOptions = (
    [
      { id: "all" as const, label: T && T.filterAll },
      { id: "airing" as const, label: T && T.filterAiring },
      { id: "finished" as const, label: T && T.filterFinished },
      { id: "standalone" as const, label: T && T.filterStandalone },
      { id: "blacklisted" as const, label: T && T.filterBlacklisted },
    ] as { id: AnimeTypeFilter; label: string }[]
  ).map((opt) => ({ ...opt, count: typeCounts[opt.id] }));

  $: selectedType = typeOptions.find((opt) => opt.id === typeFilter) ?? typeOptions[0];
  $: typeMenuItems = typeOptions.map((opt) => ({
    id: opt.id,
    label: `${opt.label} ${opt.count}`,
  }));

  function currentView(): StatusViewState {
    return { q: search, unwatched: filterUnwatched, type: typeFilter };
  }

  function applyView(view: StatusViewState) {
    search = view.q;
    filterUnwatched = view.unwatched;
    typeFilter = view.type;
  }

  function syncStatusView() {
    const qs = encodeStatusView(currentView());
    lastWrittenQuery = qs;
    replace(`/status${qs ? `?${qs}` : ""}`);
  }

  function clearFilters() {
    applyView({ q: "", unwatched: false, type: "all" });
  }

  function setTypeFilter(type: AnimeTypeFilter) {
    if (typeFilter === type) return;
    typeFilter = type;
  }

  // Incoming hash (back button, a paste, a test goto) updates the strip. Compare against the
  // query we last wrote so a replace does not loop back into applyView, and so a user click is
  // not reset before the write lands.
  $: if (viewReady) {
    const raw = $querystring ?? "";
    if (raw !== lastWrittenQuery) {
      lastWrittenQuery = raw;
      applyView(decodeStatusView(raw));
    }
  }

  $: if (viewReady) {
    search;
    filterUnwatched;
    typeFilter;
    const qs = encodeStatusView(currentView());
    if (qs !== lastWrittenQuery) {
      syncStatusView();
    }
  }

  $: nextCheckIn = status
    ? computeNextCheckIn(status.last_check, checkInterval, status.status, now)
    : null;

  $: hasReport = !!lastCheck && (lastCheck.problems.length > 0 || lastCheck.limits.length > 0);
  // The pass error beats the generic text when it exists; the generic one is still the fallback for
  // the moment between has_error arriving and the report being fetched. The sentence comes from the
  // CODE, never from Go's raw text — which used to dump AniList's response JSON into the banner.
  // The raw one is still reachable, collapsed, for whoever reports it.
  $: passErrorText = lastCheck?.pass_error_code
    ? passErrorMessage(lastCheck.pass_error_code)
    : T && T.errorAlert;
  $: passErrorRaw = lastCheck?.pass_error ?? "";
  $: onboarding = onboardingSteps(
    { completed_anime_path: completedPath, anilist_usernames: anilistUsernames, mal_usernames: malUsernames },
    animes,
    status,
  );
  // Derived from the same place as the card's item ①: a parallel `Boolean(completedPath)` doesn't
  // trim spaces, and then the card would ask for the folder while the add-anime button was already
  // enabled — the contradiction that taking the rule out of the component exists to avoid.
  $: libraryConfigured = loading || onboarding.library;
  // Title and hint of each step in the order they're numbered. Reactive because `checkInterval`
  // arrives after `getConfig()` and `$locale` can change at runtime.
  $: onboardingList = [
    {
      id: ONBOARDING_STEP_IDS[0],
      title: $locale && m.onboarding_step_library(),
      hint: $locale && m.onboarding_step_library_hint(),
    },
    {
      id: ONBOARDING_STEP_IDS[1],
      title: $locale && m.onboarding_step_source(),
      hint: $locale && m.onboarding_step_source_hint(),
    },
    {
      id: ONBOARDING_STEP_IDS[2],
      title: $locale && m.onboarding_step_check(),
      hint: $locale && m.onboarding_step_check_hint({ minutes: checkInterval }),
    },
  ];
  // Three ways out: marking the three by hand, dismissing, or the daemon already actually being
  // configured and running (then the card never shows, so as not to teach the obvious to someone
  // already using the app).
  $: showOnboarding =
    !allDone(onboarding) &&
    !$onboardingDismissed &&
    !ONBOARDING_STEP_IDS.every((id) => $onboardingDone.includes(id));

  // The server rules: disk_low is the same threshold that makes the daemon stop adding torrents.
  $: diskSpaceLow = status?.disk_low ?? false;

  $: speeds = totalSpeeds(torrents);
  $: heroSpeed = formatSpeedParts(speeds.download, fmtLocale);

  $: activeDownloads = torrents.filter((t) => t.status === "downloading");
  $: seedingCount = torrents.filter((t) => t.status === "seeding").length;

  // Library summary: the sum of the already normalized breakdowns (not of the raw fields), so the
  // aggregate obeys the same invariant watched <= downloaded <= released <= total.
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
    // `deriveAnimeChip` returns a key + raw numbers, never text (see the i18n comment in
    // lib/domain/animeState.ts). The translation happens here.
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

  // A single pass builds everything each row needs. Depends on $locale (texts), `torrents` and
  // $stallTracker (chip) and `now` ("no seeds" threshold and relative time).
  $: rows =
    $locale && sortedAnimes.map((anime) => {
      const chip = deriveAnimeChip(anime, torrents, now, $stallTracker);
      const b = breakdown(anime);
      return {
        anime,
        chip,
        chipText: chipLabel(chip),
        breakdown: b,
        // Raw ("in 2 days"), not the ready sentence: on desktop it lands in a column that already
        // has the "Next ep." label on top, and repeating it there would read "Next ep. | Next ep.
        // in 2 days". The mobile card, which has no column header, builds the whole sentence.
        nextAiringWhen: nextAiringIn(anime.next_airing_at, now, $locale),
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

  // Running and checking are "live" states — only they get the PulseDot (spec §6 restricts the
  // pulse to signs of life; stopped is a static dot).
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
    // allSettled, not all: a report failure can't take down the torrents poll, which is what feeds
    // the sparkline.
    const [torrentsResult, reportResult] = await Promise.allSettled([
      getTorrents(),
      getLastCheck(),
    ]);

    if (torrentsResult.status === "fulfilled") {
      torrents = torrentsResult.value;
      torrentsStale = false;
      // spec §8: feed the history ONLY on a successful poll. There's no "repeat the last sample" —
      // the sparkline freezing is the correct behavior, faking a continuous stream isn't.
      speedHistory.push(totalSpeeds(torrents).download);
      stallTracker.sync(torrents, Date.now());
    } else {
      torrentsStale = true;
      console.error("Failed to load torrents:", torrentsResult.reason);
    }

    if (reportResult.status === "fulfilled") {
      lastCheck = reportResult.value;
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
      completedPath = configData.completed_anime_path ?? "";
      anilistUsernames = configData.anilist_usernames ?? [];
      malUsernames = configData.mal_usernames ?? [];
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.status_toast_load_failed());
    } finally {
      loading = false;
    }
  }

  function handleWebSocketStatus(statusValue: string, lastCheck: string, hasError: boolean) {
    const previousStatus = status?.status;
    if (status) {
      status = { ...status, status: statusValue, last_check: lastCheck, has_error: hasError };
    } else {
      status = { status: statusValue, last_check: lastCheck, has_error: hasError, version: "", disk_total: 0, disk_free: 0, disk_low: false };
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
      toast.error(err instanceof Error ? err.message : m.status_toast_start_failed());
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
      toast.error(err instanceof Error ? err.message : m.status_toast_stop_failed());
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
      toast.error(err instanceof Error ? err.message : m.status_toast_check_failed());
    } finally {
      actionLoading = false;
    }
  }

  function formatDate(dateString: string) {
    if (!dateString) return m.common_never();
    const date = new Date(dateString);
    // Dates before 2010 are Go's serialized zero value, not a real date — the domain's formatDate
    // only knows how to reject invalid dates, this rule belongs to the backend.
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return m.common_never();
    return formatDateDomain(date, fmtLocale);
  }

  function formatTimeAgo(dateString: string): string {
    if (!dateString) return "";
    const date = new Date(dateString);
    if (isNaN(date.getTime()) || date.getFullYear() < 2010) return "";
    return formatRelative(date.getTime(), now, $locale);
  }


  function ringMeta(t: TorrentInfo): string {
    const eta = formatEta(t.eta_seconds, fmtLocale);
    return t.episode_number === null
      ? m.status_hero_ring_meta_batch({ eta })
      : m.status_hero_ring_meta({ episode: t.episode_number, eta });
  }

  function statusQuerystring(): string {
    const hash = window.location.hash || "";
    const q = hash.indexOf("?");
    return q >= 0 ? hash.slice(q + 1) : "";
  }

  onMount(() => {
    applyView(decodeStatusView(statusQuerystring()));
    lastWrittenQuery = encodeStatusView(currentView());
    viewReady = true;
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
    // This screen owns the poll that feeds the history; without the reset the sparkline would show
    // samples from a previous visit as if they were contiguous with the new ones.
    speedHistory.reset();
  });

  // Grid shared by the header and the list rows (desktop). Defined once so header and content can't
  // fall out of alignment.
  //
  // The fixed widths were re-measured after the type scale grew — each one is the longest text the column
  // can receive, in the widest language, rounded up:
  //   status    190px — chip "Downloading ep. 128 · 100%" = 183px (text-caption + px-2 + border)
  //   progress  290px — it's a sentence, not a number, and below that it wraps into two lines and the
  //                     row height starts to oscillate with the values. The 290px were measured for
  //                     the THREE cumulative terms caption; the four-delta one that replaced it
  //                     ("128 watched · 128 to watch · 128 to download · 128 unreleased") is ~25%
  //                     wider and wraps into two lines with 3-digit counts. It wasn't re-measured:
  //                     widening here steals from the name column, which already truncates. Instead
  //                     the anime row omits zero terms (TripleProgressBar's default), which cuts the
  //                     caption in most cases; only the Library card passes `keepZeros`
  //   last      150px — "LAST DOWNLOAD" header in text-mono-label (mono + tracking .12em)
  // The sum of the fixed columns + gaps + padding is ~754px, and that's why the table only shows from
  // `lg` (1024px) up: at `md` (768px) ~628px would be left and the columns would spill off the screen.
  // Below that the stacked cards come in.
  const LIST_GRID = "grid grid-cols-[44px_minmax(0,1fr)_190px_minmax(290px,340px)_130px_150px] items-center gap-3";
</script>

<div class="space-y-4.5">
  <!-- Header: title, daemon pill and the daemon actions (spec §9.1.1) -->
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

      <!-- `flex-wrap` and NO `shrink-0`: the row was `flex shrink-0 gap-2`, i.e. it could neither
           shrink nor wrap, and its width stayed the sum of the three buttons forever (358px in
           `en`, 438px in pt-BR). On a narrow screen "Stop Daemon" went off the right edge. The
           `shrink-0` was the problem, not the solution: as an item of the `flex-wrap` header, it
           locked the row at a single line's max-content, so the inner wrap never had a narrow line
           to work with. Without it, the floor becomes the min-content (the widest button) and the
           buttons drop to the next line. Same rule as decision 39: a CLOSED set of actions wraps,
           it doesn't scroll nor get cut. -->
      <div class="flex flex-wrap gap-2">
        <!-- The real way to the add screen (the MoreMenu item is the shortcut, not the door).
             Disabled with no library configured: it's the same block as LIBRARY_NOT_CONFIGURED,
             said BEFORE the user types a whole search only to find out nothing can be added. The
             tooltip lives on the wrapper because a disabled <a>/<button> emits no mouse events
             (NavRail.svelte's pattern). -->
        {#if libraryConfigured}
          <a
            href="#/add"
            class="inline-flex items-center justify-center gap-1.5 rounded-control bg-accent px-3 py-1.5 text-copy font-semibold text-on-accent transition-opacity hover:opacity-90"
          >
            + {$locale && m.nav_add_anime()}
          </a>
        {:else}
          <div class="tooltip tooltip-bottom" data-tip={$locale && m.add_library_required()}>
            <Button variant="ghost" disabled>+ {$locale && m.nav_add_anime()}</Button>
          </div>
        {/if}
        <Button
          variant="ghost"
          on:click={handleCheck}
          disabled={status.status === "checking" || actionLoading}
        >
          {status.status === "checking" ? (T && T.checking) : (T && T.forceCheck)}
        </Button>
        {#if status.status === "stopped"}
          <!-- The solid green is written inline, not as a Button variant, on purpose: the
               composition rule of spec §4.1 reserves the green fill EXCLUSIVELY for "Start daemon".
               An `ok` variant on the primitive would be an invitation to reuse it on another screen
               and break the "at most one solid accent" rule. -->
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
    <Loading message={(T && T.loading) || ""} />
  {:else if status}
    {#if diskSpaceLow}
      <div
        role="alert"
        class="flex items-center gap-2 rounded-field border border-danger-tint/32 bg-danger-tint/12 px-3.5 py-2.5 text-copy text-danger"
      >
        {$locale && m.status_disk_low_alert()}
      </div>
    {/if}

    {#if status.has_error && status.status !== "checking"}
      <div
        role="alert"
        class="rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
      >
        <p>{passErrorText}</p>
        {#if passErrorRaw}
          <!-- Collapsed, not hidden: the raw text is the only thing pasteable into an issue, but it
               can't be the first thing the user reads. -->
          <details class="mt-1.5">
            <summary class="cursor-pointer text-caption opacity-70 hover:opacity-100">
              {$locale && m.passerror_details()}
            </summary>
            <p class="mt-1 break-words font-mono text-caption opacity-70">{passErrorRaw}</p>
          </details>
        {/if}
      </div>
    {/if}

    <!-- First steps. An accent card (not the neutral box of the rest of the screen) because it
         competes with the hero and the report for the first glance, and loses if it looks like
         them. It disappears when the user marks the three, when they dismiss it, or when the daemon
         is already configured and running. No new request: everything comes from what
         `loadInitialData` already fetched. The dismissal is per browser (localStorage), not a
         config.json field — see decisions.md.

         Opacity in brackets (`/[.10]`): Tailwind only generates the default scale's opacities
         (…/40, /50…), so `/12` and `/28` become dead classes and the card ends up with no
         background at all. -->
    {#if showOnboarding}
      <section
        data-testid="onboarding-card"
        class="rounded-card border border-accent-tint/40 bg-accent-tint/[.10] p-4.5"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="font-mono text-mono-label uppercase text-accent">{T && T.onboardingTitle}</p>
          <!-- A TEXT button, not an "×": in a three-item card the × reads as "close until reload",
               and the behavior is permanent. The label has to say so. -->
          <button
            type="button"
            class="text-caption text-subtle underline-offset-2 transition-colors hover:text-body hover:underline"
            on:click={() => onboardingDismissed.set(true)}
          >
            {T && T.onboardingDismiss}
          </button>
        </div>

        <ol class="mt-3.5 space-y-3">
          {#each onboardingList as step, i (step.id)}
            {@const done = $onboardingDone.includes(step.id)}
            <li class="flex items-start gap-3">
              <!-- The number IS the checkbox: a separate checkbox next to a numbered badge would be
                   two markers for one state. Marking is MANUAL — deriving it from the daemon state
                   left steps 1 and 3 green on a new install, before the user had read anything. -->
              <label class="relative mt-0.5 shrink-0 cursor-pointer">
                <input
                  type="checkbox"
                  class="peer absolute inset-0 h-full w-full cursor-pointer opacity-0"
                  checked={done}
                  aria-label={step.title}
                  on:change={() => onboardingDone.toggle(step.id)}
                />
                <span
                  class="inline-flex h-5 w-5 items-center justify-center rounded-full border font-mono text-[11px] font-bold peer-focus-visible:ring-2 peer-focus-visible:ring-accent {done
                    ? 'border-accent bg-accent text-on-accent'
                    : 'border-accent-tint/40 bg-card text-accent'}"
                  aria-hidden="true">{done ? "✓" : i + 1}</span
                >
              </label>
              <div class="min-w-0 flex-1">
                <p class="text-copy font-semibold {done ? 'text-subtle line-through' : 'text-heading'}">
                  {step.title}
                </p>
                <p class="mt-0.5 text-caption text-subtle">{step.hint}</p>
                {#if !done}
                  <div class="mt-2 flex flex-wrap items-center gap-2">
                    {#if step.id === "library"}
                      <!-- `library` is already the Settings screen's initial group, hence no
                           `?group=`. -->
                      <Button href="#/config" variant="ghost">{T && T.onboardingActionConfigure}</Button>
                    {:else if step.id === "source"}
                      <!-- TWO actions, with an "or" in between: they're alternative paths, and
                           without the "or" the pair reads as two required steps. The add button
                           inherits the header's same block when there's no library. -->
                      <Button href="#/config?group=anilist" variant="ghost">
                        {T && T.onboardingActionConnectAnilist}
                      </Button>
                      <span class="text-caption text-subtle">{T && T.onboardingOr}</span>
                      {#if libraryConfigured}
                        <Button href="#/add" variant="ghost">{T && T.onboardingActionAddAnime}</Button>
                      {:else}
                        <!-- Tooltip on the wrapper: a disabled button emits no mouse events (same
                             pattern as this screen's header and NavRail). -->
                        <div class="tooltip tooltip-bottom" data-tip={$locale && m.add_library_required()}>
                          <Button variant="ghost" disabled>{T && T.onboardingActionAddAnime}</Button>
                        </div>
                      {/if}
                    {:else}
                      <Button
                        variant="ghost"
                        on:click={handleCheck}
                        disabled={status.status === "checking" || actionLoading}
                      >
                        {T && T.onboardingActionCheckNow}
                      </Button>
                    {/if}
                  </div>
                {/if}
              </div>
            </li>
          {/each}
        </ol>
      </section>
    {/if}

    <!-- The empty state is the rule: a clean pass renders nothing. A permanent card announcing "0
         problems" stops being read, and then the day it has content goes unnoticed. -->
    {#if hasReport && lastCheck}
      <!-- Two blocks, not one: a problem is a failure (amber, the same treatment as the anime
           detail's warning) and a limit is the config working as configured (neutral). Painting
           both as urgent would say having a ceiling is a defect, and the user would start ignoring
           the color. -->
      <section data-testid="last-check-report" class="space-y-3">
        {#if lastCheck.problems.length > 0}
          <div class="rounded-card border border-warn-tint/32 bg-warn-tint/12 p-4.5">
            <p class="font-mono text-mono-label uppercase text-warn">
              {T && T.reportProblems(lastCheck.problems.length)}
            </p>
            <ul class="mt-2 space-y-1.5">
              {#each lastCheck.problems as issue (`${issue.anime_id}-${issue.code}`)}
                <li class="text-copy text-heading">
                  <span class="font-semibold">{issue.anime_name}</span>
                  {#if issue.episodes && issue.episodes.length > 0}
                    <span class="text-subtle">
                      · {$locale && m.lastcheck_episodes({ episodes: issue.episodes.join(", ") })}
                    </span>
                  {/if}
                  <span class="text-warn">· {issueMessage(issue)}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if lastCheck.limits.length > 0}
          <div class="rounded-card border border-default bg-card p-4.5">
            <p class="font-mono text-mono-label uppercase text-subtle">
              {T && T.reportLimits(lastCheck.limits.length)}
            </p>
            <ul class="mt-2 space-y-1.5">
              {#each lastCheck.limits as issue (`${issue.anime_id}-${issue.code}`)}
                <li class="text-copy text-heading">
                  <span class="font-semibold">{issue.anime_name}</span>
                  <span class="text-tertiary">· {issueMessage(issue)}</span>
                  {#if batchNote(issue)}
                    <span class="text-subtle">· {batchNote(issue)}</span>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </section>
    {/if}

    <div class="grid gap-3.5 lg:grid-cols-[1.15fr_1fr]">
      <!-- Hero card: aggregate speed + sparkline + one ring per active download. `min-w-0` is NOT
           decorative: the active downloads strip below is wide (a ~190px block per torrent) and a
           grid item has `min-width: auto`, i.e. its floor is the content's intrinsic size. Without
           the override the `1.15fr` track blows up — the section grows to max-content (measured:
           2494px in a 1280px grid), the strip's `overflow-x-auto` never kicks in (width ==
           scrollWidth) and the right column is pushed off the screen. With `min-w-0` the section
           fits the track again and the strip scrolls. -->
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

        <!-- spec §8: the UI states where progress comes from. The dot turns amber when the last
             poll failed, and the text starts saying the values are frozen. -->
        <p class="mt-3 flex items-center gap-1.5 font-mono text-[12px] {torrentsStale ? 'text-warn' : 'text-subtle'}">
          <span
            class="inline-block h-1.5 w-1.5 shrink-0 rounded-full {torrentsStale ? 'bg-warn' : 'bg-neutral'}"
            aria-hidden="true"
          ></span>
          {torrentsStale ? (T && T.staleNote) : (T && T.pollingNote)}
        </p>
      </section>

      <!-- Right column: library summary + disk/next check -->
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
              keepZeros
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

    <!-- Anime list -->
    <section class="rounded-card border border-default bg-card">
      <!-- Toolbar. `shrink-0` on the container is what keeps the filter strip from collapsing when
           the list below grows (spec §9.1, mobile note). Type segments share the row with search
           from `lg` (same cut as the table); below that the ActionMenu sits next to Unwatched.
           `flex-wrap` is what stops the five Portuguese labels from painting past the card edge
           at 1024px — `overflow-x-auto` clipped them with no hint they were there. -->
      <div class="flex shrink-0 flex-col gap-3 border-b border-divider p-4 sm:flex-row sm:flex-wrap sm:items-center">
        <h2 class="text-card-title text-heading">{T && T.animesHeader}</h2>

        {#if filtersActive}
          <span class="whitespace-nowrap font-mono text-[12px] text-subtle">
            {$locale && m.status_x_of_y({ shown: filteredAnimes.length, total: animes.length })}
          </span>
          <button
            type="button"
            class="shrink-0 text-subtle hover:text-body"
            aria-label={T && T.clearFilters}
            on:click={clearFilters}
          >
            <X size={14} strokeWidth={2} />
          </button>
        {/if}

        <!-- On mobile the search takes the whole line and the filters get their own strip right
             below; stacking both in the SAME scrollable strip would push the filter chip off the
             screen, with no hint that it's there. -->
        <div class="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end">
          <label
            class="flex w-full min-w-0 items-center gap-2 rounded-field border border-neutral/45 bg-control px-2.5 py-1.5 transition-colors hover:border-neutral/70 focus-within:border-accent focus-within:ring-1 focus-within:ring-accent/40 sm:w-64 sm:shrink-0"
          >
            <Search size={16} strokeWidth={2} class="shrink-0 text-tertiary" />
            <input
              type="text"
              placeholder={(T && T.searchPlaceholder) || ""}
              bind:value={search}
              class="w-full min-w-0 bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-tertiary"
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

          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="inline-flex shrink-0 items-center gap-1.5 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {filterUnwatched
                ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
                : 'border-neutral/45 bg-control text-body hover:border-neutral/70 hover:text-heading'}"
              aria-pressed={filterUnwatched}
              on:click={() => (filterUnwatched = !filterUnwatched)}
            >
              <Eye size={14} strokeWidth={2} />
              {T && T.filterUnwatched}
              <span class="font-mono text-[12px] opacity-70">{unwatchedCount}</span>
            </button>

            <div
              class="hidden shrink-0 overflow-hidden rounded-pill border border-neutral/45 lg:inline-flex"
              role="radiogroup"
              aria-label={(T && T.filterType) || undefined}
            >
              {#each typeOptions as opt, i (opt.id)}
                <button
                  type="button"
                  role="radio"
                  aria-checked={typeFilter === opt.id}
                  class="inline-flex whitespace-nowrap items-center gap-1.5 px-3 py-1.5 text-caption font-semibold transition-colors {i > 0
                    ? 'border-l border-neutral/45'
                    : ''} {typeFilter === opt.id
                    ? 'bg-accent-tint/12 text-accent'
                    : 'bg-control text-body hover:text-heading'}"
                  on:click={() => setTypeFilter(opt.id)}
                >
                  {opt.label}
                  <span class="font-mono text-[12px] opacity-70">{opt.count}</span>
                </button>
              {/each}
            </div>

            <div class="shrink-0 lg:hidden">
              <ActionMenu
                items={typeMenuItems}
                triggerLabel={(T && T.filterType) || ""}
                triggerClass="inline-flex shrink-0 items-center gap-1.5 rounded-pill border px-3 py-1.5 text-caption font-semibold transition-colors {typeFilter !==
                'all'
                  ? 'border-accent-tint/28 bg-accent-tint/12 text-accent'
                  : 'border-neutral/45 bg-control text-body hover:border-neutral/70 hover:text-heading'}"
                on:select={(e) => setTypeFilter(e.detail as AnimeTypeFilter)}
              >
                <span slot="trigger" class="inline-flex items-center gap-1">
                  {selectedType.label}
                  <span class="font-mono text-[12px] opacity-70">{selectedType.count}</span>
                  <ChevronDown size={14} strokeWidth={2} />
                </span>
              </ActionMenu>
            </div>
          </div>
        </div>
      </div>

      {#if animes.length === 0}
        <!-- Two paths, because a list account stopped being required: adding a standalone anime is
             the real next step for someone who already has the library configured. The CTA is
             `ghost`, not solid — the screen's solid accent is already the header's "+ Add anime"
             (spec §4.1: at most one per screen). -->
        <div class="flex flex-col items-center gap-3 py-12 text-center">
          <p class="text-card-title text-body">{T && T.emptyTitle}</p>
          <p class="text-caption text-subtle">{T && T.emptyDesc}</p>
          <div class="mt-1 flex flex-wrap justify-center gap-2">
            {#if libraryConfigured}
              <a
                href="#/add"
                class="inline-flex items-center rounded-control border border-neutral/45 bg-control px-3 py-1.5 text-copy font-semibold text-heading transition-colors hover:border-accent hover:text-accent"
              >
                + {$locale && m.nav_add_anime()}
              </a>
            {/if}
            <a
              href="#/config"
              class="inline-flex items-center rounded-control border border-neutral/45 bg-control px-3 py-1.5 text-copy font-semibold text-heading transition-colors hover:border-accent hover:text-accent"
            >
              {T && T.goToConfig}
            </a>
          </div>
        </div>
      {:else if filteredAnimes.length === 0}
        <div class="py-10 text-center">
          <p class="text-copy text-subtle">{$locale && m.status_no_results({ search })}</p>
        </div>
      {:else}
        <!-- Desktop: a grid of link rows. Not a <table> on purpose — with the whole row clickable,
             one <a> per row gives the big target without needing a click handler with manual
             role/tabindex/keydown on a <tr>. -->
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
            <span class="font-mono text-mono-label uppercase text-subtle">{T && T.colNextAiring}</span>
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
              <!-- Blacklist dims the cover and title (spec §9.1), not the whole row: the
                   "Blacklisted" chip has to stay readable to explain why. -->
              <div class="h-11 w-8 overflow-hidden {row.chip.dimmed ? 'opacity-45' : ''}">
                <Cover src={row.anime.cover_image} alt={row.anime.name} />
              </div>
              <span class="truncate text-copy text-heading {row.chip.dimmed ? 'opacity-45' : ''}" title={row.anime.name}>
                {row.anime.name}
              </span>
              <!-- "Standalone" is ORIGIN, not download state: it sits NEXT TO the derived chip,
                   never inside `deriveAnimeChip` — that cascade returns a single state, and putting
                   the origin there would make a downloading standalone anime lose the "Downloading"
                   chip. -->
              <span class="flex flex-wrap items-center gap-1">
                <Chip variant={row.chip.variant} dimmed={row.chip.dimmed}>{row.chipText}</Chip>
                {#if row.anime.is_standalone}
                  <Chip variant="neutral">{$locale && m.chip_standalone()}</Chip>
                {/if}
              </span>
              <TripleProgressBar
                watched={row.breakdown.watched}
                downloaded={row.breakdown.downloaded}
                released={row.breakdown.released}
                total={row.breakdown.total}
              />
              <span class="truncate font-mono text-[12px] text-subtle">{row.nextAiringWhen}</span>
              <span class="flex items-center justify-between gap-1 font-mono text-[12px] text-subtle">
                {row.lastDownload}
                {#if row.href}
                  <ChevronRight size={14} strokeWidth={2} class="shrink-0" />
                {/if}
              </span>
            </svelte:element>
          {/each}
        </div>

        <!-- Mobile/tablet: the same data stacked in cards (see the widths note in LIST_GRID on why
             the cut is at `lg` and not `md`) -->
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
                  <div class="mt-1.5 flex flex-wrap items-center gap-1">
                    <Chip variant={row.chip.variant} dimmed={row.chip.dimmed}>{row.chipText}</Chip>
                    {#if row.anime.is_standalone}
                      <Chip variant="neutral">{$locale && m.chip_standalone()}</Chip>
                    {/if}
                  </div>
                  <div class="mt-2">
                    <TripleProgressBar
                      watched={row.breakdown.watched}
                      downloaded={row.breakdown.downloaded}
                      released={row.breakdown.released}
                      total={row.breakdown.total}
                    />
                  </div>
                  <p class="mt-1.5 font-mono text-[12px] text-subtle">
                    {row.lastDownload}{#if row.nextAiringWhen} · {m.next_airing({ when: row.nextAiringWhen })}{/if}
                  </p>
                </div>
              </div>
            </svelte:element>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>
