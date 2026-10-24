package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func twoPacksWEBThenBD() []nyaa.TorrentResult {
	r1080 := "1080p"
	return []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime 01-12 (1080p) WEB-DL", MagnetLink: fakeMagnet(1), Resolution: &r1080, Seeders: "400"},
		{Name: "[Group] Anime 01-12 [BD] [1080p]", MagnetLink: fakeMagnet(2), Resolution: &r1080, Seeders: "60"},
	}
}

func TestProcessAnimeEpisodes_FinishedPrefersBDPack(t *testing.T) {
	anime := animeWithEpisodes(12, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor(twoPacksWEBThenBD(), nil, nil, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) == 0 {
		t.Fatal("expected a pack download")
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0002") {
		t.Fatalf("FINISHED must pick the BD pack, hash %s", result.newEpisodes[0].EpisodeHash)
	}
}

func TestProcessAnimeEpisodes_ReleasingKeepsScrapeOrder(t *testing.T) {
	anime := animeWithEpisodes(12, anilist.MediaStatusReleasing, true, "")
	searcher := searcherFor(twoPacksWEBThenBD(), nil, nil, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) == 0 {
		t.Fatal("expected a pack download")
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0001") {
		t.Fatalf("RELEASING must keep scrape order (WEB first), hash %s", result.newEpisodes[0].EpisodeHash)
	}
}

func TestRankSeriesResults_SingleEpisodeFallback(t *testing.T) {
	r1080 := "1080p"
	ep := 1
	singles := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 01 (1080p) WEB-DL", MagnetLink: fakeMagnet(1), Resolution: &r1080, Seeders: "400", Episode: &ep},
		{Name: "[Group] Anime - 01 [BD] [1080p]", MagnetLink: fakeMagnet(2), Resolution: &r1080, Seeders: "60", Episode: &ep},
	}
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor(nil, nil, singles, nil)
	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, searcher, nil)
	if len(result.newEpisodes) != 1 {
		t.Fatalf("want 1 episode, got %d", len(result.newEpisodes))
	}
	if !strings.HasSuffix(result.newEpisodes[0].EpisodeHash, "0002") {
		t.Fatalf("FINISHED single fallback must pick BD, hash %s", result.newEpisodes[0].EpisodeHash)
	}
}
