package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// An AniList account is no longer required: with standalone animes the app works fully with
// no list configured. The only remaining requirement is the library — without it there is
// nowhere to download to.
func TestIsConfigComplete_WithoutAniListAccount(t *testing.T) {
	config := &files.Config{
		AnilistUsernames:   []string{},
		CompletedAnimePath: "/tmp/completed",
	}

	if !isConfigComplete(config) {
		t.Fatal("config with a library but no AniList account must be complete")
	}
}

func TestIsConfigComplete_WithoutLibrary(t *testing.T) {
	config := &files.Config{AnilistUsernames: []string{"user1"}}

	if isConfigComplete(config) {
		t.Fatal("config without a library cannot be complete: there is nowhere to download to")
	}
}

// TestSearchAnilist_NoAccountsStillProcessesStandalone: with no account the pass still has
// work to do — the standalones. Returning an error here would abort the whole pass and the
// feature would never run on an install without AniList.
func TestSearchAnilist_NoAccountsStillProcessesStandalone(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	configs := standaloneTestConfig()
	configs.AnilistUsernames = []string{}

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, configs, []int{500})
	if err != nil {
		t.Fatalf("no account is not an error: %v", err)
	}
	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 500 {
		t.Fatalf("the standalone must be processed, got %+v", resp.Data.Page.MediaList)
	}
}

// TestSearchAnilist_NoLibraryStillFails: the library is still required.
func TestSearchAnilist_NoLibraryStillFails(t *testing.T) {
	configs := standaloneTestConfig()
	configs.CompletedAnimePath = ""

	if _, err := searchAnilist(&mockFileManagerForEpisodes{}, configs, nil); err == nil {
		t.Fatal("want an error when no library is configured")
	}
}

// TestManualDownloadEpisode_StandaloneAnime: a standalone's detail screen shows the
// download/redownload/replace buttons per episode. Without the GetMediaByID fallback in
// resolveAnimeDetails they all fail before reaching Nyaa — the screen opens and no button
// works.
func TestManualDownloadEpisode_StandaloneAnime(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()
	defer mockEmptyNyaa()()

	configs := standaloneTestConfig()
	configs.AnilistUsernames = []string{}

	// With no torrent on Nyaa the download fails, but the message has to be about the torrent —
	// not about the anime not being on any list.
	_, err := ManualDownloadEpisode(&mockFileManagerForEpisodes{}, torrents.NewFakeBackend(), 500, 1, configs)
	if err == nil {
		t.Fatal("with no torrent on Nyaa the download must fail")
	}
	if !strings.Contains(err.Error(), "no torrents found") {
		t.Fatalf("the standalone anime must be resolved; error came from somewhere else: %v", err)
	}
}
