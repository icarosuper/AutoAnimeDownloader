package daemon

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

// animeWithEpisodes builds an anime with n aired episode nodes. total is what AniList reports as
// Media.Episodes (nil when totalKnown is false).
func animeWithEpisodes(n int, status anilist.MediaStatus, totalKnown bool, format anilist.MediaFormat) anilist.MediaList {
	title := "Limits Test Anime"
	nodes := make([]anilist.AiringNode, n)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 2000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	media := anilist.Media{
		Id:             777,
		Status:         status,
		Format:         format,
		Title:          anilist.Title{English: &title},
		AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
	}
	if totalKnown {
		total := n
		media.Episodes = &total
	}
	return anilist.MediaList{Id: 777, Status: anilist.MediaListStatusCurrent, Media: media}
}

// searcherFor builds a searcher whose strategies return the given results. batch and multiple
// become ONE list (the anime search returns both together): a pack gets IsBatch, an episode already
// comes with Episode filled by multipleFor.
func searcherFor(batch, multiple, single, movie []nyaa.TorrentResult) nyaaSearcher {
	anime := make([]nyaa.TorrentResult, 0, len(batch)+len(multiple))
	for _, tr := range batch {
		tr.IsBatch = true
		anime = append(anime, tr)
	}
	anime = append(anime, multiple...)

	return nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int) []nyaa.TorrentResult { return anime },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, int) []nyaa.TorrentResult {
			return single
		},
		searchMovie: func(anilist.Title, bool) []nyaa.TorrentResult { return movie },
	}
}

// fakeMagnet builds a magnet with a valid (unique) 40-hex infohash, which FakeBackend requires.
func fakeMagnet(n int) string {
	return "magnet:?xt=urn:btih:" + strings.Repeat("0", 36) + fmt.Sprintf("%04x", n)
}

// multipleFor builds one per-episode result for each episode number in 1..n, as the singles
// side of ScrapNyaaForAnime would.
func multipleFor(n int, sizeBytes int64) []nyaa.TorrentResult {
	out := make([]nyaa.TorrentResult, 0, n)
	for i := 1; i <= n; i++ {
		ep := i
		out = append(out, nyaa.TorrentResult{
			Name:       "ep",
			MagnetLink: fakeMagnet(i),
			Episode:    &ep,
			Size:       sizeBytes,
		})
	}
	return out
}

func limitsConfig() *files.Config {
	return &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   3,
	}
}

// TestWillBatch_FinishedWithinCeilingIgnoresPerAnimeLimit: 26 finished episodes match the batch,
// and the per-anime limit of 12 does NOT apply — a batch is a single torrent, limiting records
// would limit neither bytes nor files in the library.
func TestWillBatch_FinishedWithinCeilingIgnoresPerAnimeLimit(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "batch", MagnetLink: fakeMagnet(9001)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 26 {
		t.Errorf("want 26 episodes recorded by the batch, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episode %d should be marked as batch", ep.EpisodeNumber)
		}
	}
}

// RELEASING with 1100 episodes SEARCHES for a pack (before this spec it didn't, by status), and the
// partial pack covers the per-anime limit window.
func TestEligibility_ReleasingLongSeriesUsesPartialPack(t *testing.T) {
	anime := animeWithEpisodes(1100, anilist.MediaStatusReleasing, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 001-100 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 100 {
		t.Fatalf("want the pack's 100 episodes recorded, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episode %d should be marked as batch", ep.EpisodeNumber)
		}
	}
}

// Media.Episodes == nil becomes eligible: with no count comparison, an unknown count stops
// mattering.
func TestEligibility_UnknownTotalIsEligible(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 26 {
		t.Errorf("an unknown count must be able to use a pack, got %d", len(result.newEpisodes))
	}
}

// One pending episode doesn't search for a pack: a pack for one episode is the loose-episode path.
func TestEligibility_SinglePendingEpisodeDoesNotUsePack(t *testing.T) {
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, multipleFor(1, 0), nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 1 || result.newEpisodes[0].IsBatch {
		t.Errorf("want 1 loose episode, got %+v", result.newEpisodes)
	}
}

// The path the stopgap covered, now without it: the size filter empties the packs, pickBatches
// returns empty, the flow falls to loose episodes and the per-anime limit APPLIES.
func TestEligibility_SizeCeilingEmptiesPacksAndLimitApplies(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	configs := limitsConfig()
	configs.MaxBatchTorrentSizeGB = 10
	const gib = int64(1024 * 1024 * 1024)
	searcher := searcherFor(
		[]nyaa.TorrentResult{{Name: "[X] Anime 01-26 remux [1080p]", MagnetLink: fakeMagnet(9002), Size: 300 * gib}},
		multipleFor(26, gib),
		nil, nil,
	)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 12 {
		t.Errorf("want 12 individual episodes, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatal("no episode should come marked as batch")
		}
	}
}

// One Piece shape: RELEASING, Media.Episodes == nil, 1100 episodes aired. The series length has to
// reach the episode search (it's what turns on the query's zero-padding) — and Media.Episodes alone
// wouldn't do, because it's precisely nil on the long airing series.
func TestSingleEpisodeSearch_ReceivesSeriesLength(t *testing.T) {
	anime := animeWithEpisodes(1100, anilist.MediaStatusReleasing, false, "")
	got := 0
	searcher := searcherFor(nil, nil, nil, nil)
	searcher.searchSingleEpisode = func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, totalEpisodes int) []nyaa.TorrentResult {
		got = totalEpisodes
		return nil
	}

	processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if got != 1100 {
		t.Errorf("want 1100 as the series length in the episode search, got %d", got)
	}
}

// TestWillBatch_MovieUnaffected: a movie keeps using the movie strategy.
func TestWillBatch_MovieUnaffected(t *testing.T) {
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, anilist.MediaFormatMovie)
	searcher := searcherFor(nil, nil, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(9004)}})

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 1 || !result.newEpisodes[0].IsBatch {
		t.Errorf("a movie must download as a single torrent, got %+v", result.newEpisodes)
	}
}

// TestEpisodeSizeCeiling_DoesNotAffectBatchChoice: a 40 GiB pack is still downloaded with
// max_episode_torrent_size_gb = 1.5 — the two ceilings are independent.
func TestEpisodeSizeCeiling_DoesNotAffectBatchChoice(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	configs := limitsConfig()
	configs.MaxEpisodeTorrentSizeGB = 1.5
	const gib = int64(1024 * 1024 * 1024)
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "pack", MagnetLink: fakeMagnet(9003), Size: 40 * gib}}, nil, nil, nil)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 26 {
		t.Errorf("the episode ceiling must not filter the batch, got %d episodes", len(result.newEpisodes))
	}
}

func TestFilterBySize(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	results := []nyaa.TorrentResult{
		{Name: "small", Size: 1 * gib},
		{Name: "huge", Size: 300 * gib},
		{Name: "unknown", Size: 0},
		{Name: "medium", Size: 5 * gib},
	}

	if got, dropped := filterBySize(results, 0); len(got) != 4 || dropped != 0 {
		t.Errorf("ceiling 0 must filter nothing, %d left and %d dropped", len(got), dropped)
	}

	got, dropped := filterBySize(results, 10)
	if len(got) != 3 {
		t.Fatalf("want 3 results, got %d (%+v)", len(got), got)
	}
	if dropped != 1 {
		t.Errorf("want 1 dropped by size, got %d", dropped)
	}
	// Size == 0 stays (broken parsing can't become a standstill) and the order is kept.
	want := []string{"small", "unknown", "medium"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: want %q, got %q", i, name, got[i].Name)
		}
	}
}

func TestFilterBySeeders(t *testing.T) {
	results := []nyaa.TorrentResult{
		{Name: "alive", Seeders: "412"},
		{Name: "dead", Seeders: "0"},
		{Name: "unknown", Seeders: "-"},
		{Name: "weak", Seeders: "3"},
	}

	if got, dropped := filterBySeeders(results, 0); len(got) != 4 || dropped != 0 {
		t.Errorf("floor 0 must filter nothing, %d left and %d dropped", len(got), dropped)
	}

	// Default: only the literally dead one goes (and "-" counts as 0).
	got, dropped := filterBySeeders(results, 1)
	want := []string{"alive", "weak"}
	if len(got) != len(want) {
		t.Fatalf("want %d results, got %d (%+v)", len(want), len(got), got)
	}
	if dropped != 2 {
		t.Errorf("want 2 dropped by seeders, got %d", dropped)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: want %q, got %q", i, name, got[i].Name)
		}
	}

	if got, _ := filterBySeeders(results, 5); len(got) != 1 || got[0].Name != "alive" {
		t.Errorf("floor 5 should leave only alive, got %+v", got)
	}
}

// TestFilterSearchResults_DropStats: dropStats is the only thing that tells "Nyaa returned nothing"
// from "the filter cut everything", and the last-verification report depends on that distinction so
// it doesn't lie saying "no torrent found" when there were eight.
func TestFilterSearchResults_DropStats(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	t.Run("empty input", func(t *testing.T) {
		got, stats := filterSearchResults(nil, 3, 1)
		if len(got) != 0 {
			t.Errorf("want an empty list, got %+v", got)
		}
		if stats != (dropStats{}) {
			t.Errorf("want zeroed dropStats, got %+v", stats)
		}
	})

	t.Run("cut by size only", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "huge-a", Size: 8 * gib, Seeders: "50"},
			{Name: "huge-b", Size: 9 * gib, Seeders: "40"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 0 {
			t.Fatalf("want an empty list, got %+v", got)
		}
		want := dropStats{Input: 2, BySize: 2, BySeeders: 0}
		if stats != want {
			t.Errorf("want %+v, got %+v", want, stats)
		}
	})

	t.Run("cut by seeders only", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "dead-a", Size: 1 * gib, Seeders: "0"},
			{Name: "dead-b", Size: 1 * gib, Seeders: "0"},
			{Name: "dead-c", Size: 1 * gib, Seeders: "0"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 0 {
			t.Fatalf("want an empty list, got %+v", got)
		}
		want := dropStats{Input: 3, BySize: 0, BySeeders: 3}
		if stats != want {
			t.Errorf("want %+v, got %+v", want, stats)
		}
	})

	t.Run("cut by both", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "huge", Size: 8 * gib, Seeders: "50"},
			{Name: "dead", Size: 1 * gib, Seeders: "0"},
			{Name: "ok", Size: 1 * gib, Seeders: "50"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 1 || got[0].Name != "ok" {
			t.Fatalf("want only ok, got %+v", got)
		}
		want := dropStats{Input: 3, BySize: 1, BySeeders: 1}
		if stats != want {
			t.Errorf("want %+v, got %+v", want, stats)
		}
	})
}

// --- Disk space guard ---

func TestCheckDiskSpace(t *testing.T) {
	dir := t.TempDir()
	_, free, err := files.DiskSpace(dir)
	if err != nil || free == 0 {
		t.Skipf("no free-space reading for the temp dir: %v", err)
	}

	if free >= uint64(giB) {
		if err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskGB: 1}, 0, 0); err != nil {
			t.Errorf("with 1 GiB required want nil, got %v", err)
		}
		if err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskGB: 1}, 1, 0); err != nil {
			t.Errorf("a 1-byte torrent must fit, got %v", err)
		}
	}
	// A floor larger than what's free always trips the policy.
	err = checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskGB: int(free>>30) + 1}, 0, 0)
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("want ErrInsufficientDiskSpace, got %v", err)
	}
	if err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskGB: 0}, 0, 0); err != nil {
		t.Errorf("0 turns the policy off, got %v", err)
	}
	// Size larger than the whole volume: blocked even with the GiB policy off.
	err = checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskGB: 0}, math.MaxInt64, 0)
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("a torrent larger than the disk must be blocked even with the guard off, got %v", err)
	}
	// A statfs error doesn't block.
	missing := filepath.Join(dir, "does-not-exist")
	if err := checkDiskSpace(&files.Config{CompletedAnimePath: missing, MinFreeDiskGB: 100}, 0, 0); err != nil {
		t.Errorf("a statfs failure must not block, got %v", err)
	}
}

func TestCheckDiskSpace_ReservedAloneTripsTheGiBFloor(t *testing.T) {
	dir := t.TempDir()
	_, free, err := files.DiskSpace(dir)
	if err != nil || free < 3*uint64(giB) {
		t.Skipf("need a few GiB free to split between reserved and leftover: %v", err)
	}
	minGB := 10
	if free < uint64(minGB)*uint64(giB)+2*uint64(giB) {
		minGB = max(1, int(free>>30)/2)
	}
	reserved := int64(free) - int64(minGB-1)*giB
	cfg := &files.Config{CompletedAnimePath: dir, MinFreeDiskGB: minGB}
	if err := checkDiskSpace(cfg, 0, 0); err != nil {
		t.Fatalf("without reserved the floor must pass, got %v", err)
	}
	if err := checkDiskSpace(cfg, 0, reserved); !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("reserved alone must trip the GiB floor, got %v", err)
	}
}

func TestCheckDiskSpace_PolicyOffStillRefusesBiggerThanFreeMinusReserved(t *testing.T) {
	dir := t.TempDir()
	_, free, err := files.DiskSpace(dir)
	if err != nil || free < 4 {
		t.Skipf("no free-space reading for the temp dir: %v", err)
	}
	cfg := &files.Config{CompletedAnimePath: dir, MinFreeDiskGB: 0}
	reserved := int64(free) / 2
	if err := checkDiskSpace(cfg, int64(free)-reserved+1, reserved); !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("want refuse when size+reserved > free with policy off, got %v", err)
	}
	if reserved > 0 {
		if err := checkDiskSpace(cfg, reserved-1, reserved); err != nil {
			t.Errorf("size+reserved still under free must pass with policy off, got %v", err)
		}
	}
}

// diskFullConfig returns a config whose disk guard always blocks.
func diskFullConfig(t *testing.T) *files.Config {
	t.Helper()
	configs := limitsConfig()
	configs.CompletedAnimePath = t.TempDir()
	_, free, err := files.DiskSpace(configs.CompletedAnimePath)
	if err != nil {
		t.Fatal(err)
	}
	configs.MinFreeDiskGB = int(free>>30) + 1
	if configs.MinFreeDiskGB < 1 {
		configs.MinFreeDiskGB = 1
	}
	return configs
}

func TestAttemptDownloadWithRetries_DiskFullDoesNotCallAdd(t *testing.T) {
	backend := torrents.NewFakeBackend()
	hash, _ := attemptDownloadWithRetries(diskFullConfig(t), backend, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(1)}, {MagnetLink: fakeMagnet(2)}}, "ep")

	if hash != "" {
		t.Errorf("want an empty hash with a full disk, got %q", hash)
	}
	if len(backend.List()) != 0 {
		t.Errorf("backend.Add must never be called, the client has %d torrent(s)", len(backend.List()))
	}
}

func TestProcessAnimeEpisodes_DiskFullNotifiesNoDiskSpaceReason(t *testing.T) {
	bodies := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
	}))
	defer srv.Close()

	configs := diskFullConfig(t)
	configs.Notifications = files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
		Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
		Body: "{{reason}}", Events: []string{"download_failed"},
	}}}

	anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, false, "")
	searcher := searcherFor(nil, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(1)}}, nil)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) != 0 {
		t.Errorf("nothing must be recorded with a full disk, got %d", len(result.newEpisodes))
	}

	select {
	case body := <-bodies:
		if !strings.Contains(body, notifications.ReasonNoDiskSpace) {
			t.Errorf("want reason %q in the webhook, got %q", notifications.ReasonNoDiskSpace, body)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("no failure webhook received")
	}
}

// TestHandleSavedEpisodes_DiskFullStillPrunes: a full disk blocks the Add, not the pass — pruning
// is precisely what frees space.
func TestHandleSavedEpisodes_DiskFullStillPrunes(t *testing.T) {
	configs := diskFullConfig(t)
	configs.DeleteWatchedEpisodes = true

	saved := []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: "h1"}, {EpisodeNumber: 2, EpisodeHash: "h2"}}
	fm := &mockFileManagerForEpisodes{}
	backend := fakeWithTorrents("h1", "h2")

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes: saved,
		keysToDelete:  []files.EpisodeKey{{Episode: 1}, {Episode: 2}},
	})

	if !containsID(fm.deletedEpisodeKeys, files.EpisodeKey{Episode: 1}) || !containsID(fm.deletedEpisodeKeys, files.EpisodeKey{Episode: 2}) {
		t.Errorf("pruning must run with a full disk, deleted = %v", fm.deletedEpisodeKeys)
	}
}

// TestOrganizeTorrent_PartiallyOrganizedSkipsWebhook: new records created for an already organized
// torrent (the batch-rule migration path) don't re-fire DownloadCompleted.
func TestOrganizeTorrent_PartiallyOrganizedSkipsWebhook(t *testing.T) {
	events := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- string(body)
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "ep01.mkv"), []byte("v"), 0644); err != nil {
		t.Fatal(err)
	}
	const hash = "0123456789abcdef0123456789abcdef01234567"
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)

	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 1, IsBatch: true, LibraryPaths: []string{"/library/My Anime/ep01.mkv"}},
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 2, IsBatch: true},
		},
		configs: &files.Config{
			CompletedAnimePath: t.TempDir(),
			Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
				Body: "done", Events: []string{"download_completed"},
			}}},
		},
	}

	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent should succeed")
	}
	if len(fm.upserted) != 1 {
		t.Errorf("the LibraryPaths marker must be written to the new records, upserts = %d", len(fm.upserted))
	}
	select {
	case body := <-events:
		t.Errorf("no webhook should go out for an already organized torrent, got %q", body)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestProcessAnimeEpisodes_Issues: the reasons come out through processAnimeEpisodes, in the same
// channel that already returns newEpisodes/checkedEpisodes/keysToDelete. No global collector, no
// new mutex.
func TestProcessAnimeEpisodes_Issues(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	t.Run("all candidates above the ceiling", func(t *testing.T) {
		anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:      t.TempDir(),
			MaxEpisodesPerAnime:     12,
			EpisodeRetryLimit:       3,
			MaxEpisodeTorrentSizeGB: 3,
			MinSeeders:              1,
		}
		big := []nyaa.TorrentResult{
			{Name: "huge-a", Size: 8 * gib, Seeders: "50", MagnetLink: fakeMagnet(8001)},
			{Name: "huge-b", Size: 9 * gib, Seeders: "40", MagnetLink: fakeMagnet(8002)},
		}
		searcher := searcherFor(nil, nil, big, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, nil, searcher, nil)

		if len(result.issues) != 1 {
			t.Fatalf("want 1 issue, got %d (%+v)", len(result.issues), result.issues)
		}
		got := result.issues[0]
		if got.Code != IssueAllAboveSizeLimit {
			t.Errorf("want %q, got %q", IssueAllAboveSizeLimit, got.Code)
		}
		if got.Candidates != 2 || got.LimitGB != 3 {
			t.Errorf("wrong details: %+v", got)
		}
		if got.AnimeID != anime.Media.Id || got.AnimeName == "" {
			t.Errorf("anime identification missing: %+v", got)
		}
	})

	t.Run("empty search becomes no_torrent_found", func(t *testing.T) {
		anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:  t.TempDir(),
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   3,
			MinSeeders:          1,
		}
		searcher := searcherFor(nil, nil, nil, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, nil, searcher, nil)

		if len(result.issues) != 1 || result.issues[0].Code != IssueNoTorrentFound {
			t.Fatalf("want a no_torrent_found, got %+v", result.issues)
		}
	})

	t.Run("per-anime limit with batch without result", func(t *testing.T) {
		anime := animeWithEpisodes(10, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:  t.TempDir(),
			MaxEpisodesPerAnime: 3,
			EpisodeRetryLimit:   3,
			MinSeeders:          1,
		}
		singles := make([]nyaa.TorrentResult, 3)
		for i := range singles {
			ep := i + 1
			singles[i] = nyaa.TorrentResult{Name: "ep", Size: gib, Seeders: "50", MagnetLink: fakeMagnet(7000 + i), Episode: &ep}
		}
		searcher := searcherFor(nil, singles, nil, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, nil, searcher, nil)

		var limit *Issue
		for i := range result.issues {
			if result.issues[i].Code == IssueMaxEpisodesPerAnime {
				limit = &result.issues[i]
			}
		}
		if limit == nil {
			t.Fatalf("want a max_episodes_per_anime, got %+v", result.issues)
		}
		if limit.Downloaded != 3 || limit.Pending != 7 {
			t.Errorf("want downloaded=3 pending=7, got %+v", limit)
		}
		if limit.BatchSkipped != BatchSkippedNoResult {
			t.Errorf("want batch_skipped=%q, got %q", BatchSkippedNoResult, limit.BatchSkipped)
		}
		if len(limit.Episodes) != 0 {
			t.Errorf("the limit carries no episode numbers, got %+v", limit.Episodes)
		}
	})
}

// Flower and Asura: the only pack covering the window didn't fit on disk, and the guard used to
// run only at download time — after the pack owned the window — so no episode got a candidate.
// A pack that doesn't fit now leaves the window to the loose episodes.
func TestPackThatDoesNotFitFallsBackToLooseEpisodes(t *testing.T) {
	anime := animeWithEpisodes(12, anilist.MediaStatusFinished, true, "")
	huge := nyaa.TorrentResult{Name: "[X] Anime 01-12 [BD Remux]", MagnetLink: fakeMagnet(9001), Size: math.MaxInt64}
	searcher := searcherFor([]nyaa.TorrentResult{huge}, multipleFor(12, 1), nil, nil)
	configs := limitsConfig()
	configs.CompletedAnimePath = t.TempDir()

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)

	if len(result.newEpisodes) != 12 {
		t.Fatalf("want the 12 loose episodes, got %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatalf("episode %d came from the pack that doesn't fit", ep.EpisodeNumber)
		}
	}
}

// The #101 race: packsThatFit is a snapshot; another anime of the same pass can spend the
// budget before this one Adds. The pack that then loses at reserve must not keep the window —
// the next pack or the loose episodes take over, same as when packsThatFit dropped it up front.
func TestPackThatLosesReserveFallsBackToSingles(t *testing.T) {
	configs, size := budgetConfig(t)
	disk := newDiskBudget(nil, nil)
	anime := animeWithEpisodes(12, anilist.MediaStatusFinished, true, "")
	pack := nyaa.TorrentResult{Name: "[X] Anime 01-12 [1080p]", MagnetLink: fakeMagnet(9001), Size: size}
	searcher := searcherFor([]nyaa.TorrentResult{pack}, multipleFor(12, 1), nil, nil)

	var once sync.Once
	afterPacksThatFit = func() {
		once.Do(func() {
			if _, err := disk.reserve(configs, nyaa.TorrentResult{MagnetLink: fakeMagnet(1), Size: size}); err != nil {
				t.Errorf("filling the budget between packsThatFit and Add: %v", err)
			}
		})
	}
	defer func() { afterPacksThatFit = nil }()

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, disk)

	if len(result.newEpisodes) != 12 {
		t.Fatalf("want the 12 singles after the pack lost the budget, got %d (issues=%+v)", len(result.newEpisodes), result.issues)
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatalf("episode %d came from the pack that no longer fitted", ep.EpisodeNumber)
		}
	}
}

func TestPackThatLosesReserveFallsBackToNextPack(t *testing.T) {
	configs, size := budgetConfig(t)
	disk := newDiskBudget(nil, nil)
	anime := animeWithEpisodes(12, anilist.MediaStatusFinished, true, "")
	big := nyaa.TorrentResult{Name: "[X] Anime 01-12 [BD Remux]", MagnetLink: fakeMagnet(9001), Size: size}
	small := nyaa.TorrentResult{Name: "[Y] Anime 01-12 [1080p]", MagnetLink: fakeMagnet(9002), Size: 1}
	searcher := searcherFor([]nyaa.TorrentResult{big, small}, nil, nil, nil)

	var once sync.Once
	afterPacksThatFit = func() {
		once.Do(func() {
			if _, err := disk.reserve(configs, nyaa.TorrentResult{MagnetLink: fakeMagnet(1), Size: size}); err != nil {
				t.Errorf("filling the budget between packsThatFit and Add: %v", err)
			}
		})
	}
	defer func() { afterPacksThatFit = nil }()

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, disk)

	if len(result.newEpisodes) != 12 {
		t.Fatalf("want the 12 episodes of the smaller pack, got %d (issues=%+v)", len(result.newEpisodes), result.issues)
	}
	wantHash, err := torrents.InfoHash(small.MagnetLink)
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episode %d fell through to a single; want the next pack", ep.EpisodeNumber)
		}
		if ep.EpisodeHash != wantHash {
			t.Fatalf("episode %d hash %q, want the smaller pack %q", ep.EpisodeNumber, ep.EpisodeHash, wantHash)
		}
	}
}

// TestEmptyEpisodeSearchWaitsAndStopsTheLaterOnes: a per-episode search that comes back empty isn't
// repeated on the next pass, and the episodes after it skip the search in the same pass — but keep
// their report row. Once the wait is over and the episode shows up, the later ones are searched again.
func TestEmptyEpisodeSearchWaitsAndStopsTheLaterOnes(t *testing.T) {
	anime := animeWithEpisodes(3, anilist.MediaStatusFinished, true, "")
	var searched []int
	found := map[int]bool{}
	searcher := searcherFor(nil, nil, nil, nil)
	searcher.misses = &searchMisses{byKey: map[files.EpisodeKey]searchMiss{}}
	searcher.searchSingleEpisode = func(ep anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ int) []nyaa.TorrentResult {
		searched = append(searched, ep.Episode)
		if !found[ep.Episode] {
			return nil
		}
		n := ep.Episode
		return []nyaa.TorrentResult{{Name: "ep", MagnetLink: fakeMagnet(n), Episode: &n, Seeders: "10"}}
	}
	pass := func() animeProcessResult {
		searched = nil
		return processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	}

	result := pass()
	if fmt.Sprint(searched) != "[1]" || len(result.issues) != 3 {
		t.Fatalf("first pass: want only episode 1 searched and 3 issues, got %v and %d", searched, len(result.issues))
	}
	// Only episode 1 was searched: 2 and 3 must not claim "no torrent found".
	var codes []string
	for _, issue := range result.issues {
		codes = append(codes, issue.Code)
	}
	if want := fmt.Sprint([]string{IssueNoTorrentFound, IssueAwaitingEarlierEpisode, IssueAwaitingEarlierEpisode}); fmt.Sprint(codes) != want {
		t.Errorf("first pass: want codes %s, got %v", want, codes)
	}
	if pass(); len(searched) != 0 {
		t.Fatalf("second pass: want no search while episode 1 waits, got %v", searched)
	}

	miss := searcher.misses.byKey[files.EpisodeKey{AnimeID: 777, Episode: 1}]
	miss.retryAt = time.Now().Add(-time.Second)
	searcher.misses.byKey[files.EpisodeKey{AnimeID: 777, Episode: 1}] = miss
	found[1], found[2] = true, true
	if result := pass(); fmt.Sprint(searched) != "[1 2 3]" || len(result.newEpisodes) != 2 {
		t.Fatalf("after the wait: want episodes 1-3 searched and 2 downloaded, got %v and %d", searched, len(result.newEpisodes))
	}
}
