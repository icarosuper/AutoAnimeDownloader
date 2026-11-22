package daemon

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// mockMAL serves a MAL list (load.json body) and maps every MAL id to AniList id + 1000.
// malStatus non-zero makes MAL answer that status code instead.
func mockMAL(t *testing.T, list string, malStatus int) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		reply := func(status int, body string) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		if strings.Contains(req.URL.Host, "myanimelist") {
			if malStatus != 0 {
				return reply(malStatus, `{"errors":[{"message":"invalid request"}]}`)
			}
			return reply(http.StatusOK, list)
		}
		body, _ := io.ReadAll(req.Body)
		if strings.Contains(string(body), "idMal_in") {
			return reply(http.StatusOK, `{"data":{"Page":{"media":[{"id":1001,"idMal":1}]}}}`)
		}
		t.Errorf("unexpected AniList query: %s", body)
		return nil, fmt.Errorf("unexpected call")
	})
}

func malDeleteConfig() *files.Config {
	return &files.Config{
		AnilistUsernames: []string{"accountA"},
		MALUsernames:     []string{"malUser"},
		DeleteStatuses:   []string{"COMPLETED"},
	}
}

// The AND rule of decisions/043 holds for a MAL account: still watching on MAL vetoes the
// deletion an AniList account asked for.
func TestDeletableMediaIDs_MALAccountWatchingVetoes(t *testing.T) {
	got := deletableMediaIDs(malDeleteConfig(), map[string]statuses{
		"accountA":    {1001: "COMPLETED"},
		"mal:malUser": {1001: "CURRENT"},
	}, []int{1001})

	if got[1001] {
		t.Error("a MAL account still watching the anime must veto its deletion")
	}
}

// fetchListStatuses turns the MAL list into the same per-account map, limited to the animes on
// disk: an anime the MAL account doesn't have doesn't vote.
func TestFetchListStatuses_MALAccountWithoutTheAnimeDoesNotVeto(t *testing.T) {
	defer mockMAL(t, `[{"anime_id":1,"status":1,"num_watched_episodes":3,"is_rewatching":0}]`, 0)()

	cfg := malDeleteConfig()
	cfg.AnilistUsernames = nil
	saved := []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}, {EpisodeNumber: 1, AnimeID: 1001}}
	got := fetchListStatuses(cfg, onDiskMediaIDs(saved))

	if got["mal:malUser"][1001] != anilist.MediaListStatusCurrent {
		t.Fatalf("want the MAL entry mapped to 1001 as CURRENT, got %v", got["mal:malUser"])
	}
	if _, tracked := got["mal:malUser"][500]; tracked {
		t.Error("an anime the MAL list doesn't have must be absent")
	}
	got["accountA"] = statuses{500: "COMPLETED"}
	cfg.AnilistUsernames = []string{"accountA"}
	if !deletableMediaIDs(cfg, got, onDiskMediaIDs(saved))[500] {
		t.Error("a MAL account that doesn't track the anime must not veto")
	}
}

// A MAL list that came back empty (anilist.ErrMALEmpty) is a failure: the account is absent, and
// without its vote there is no unanimity.
func TestFetchListStatuses_FailedMALFetchVetoes(t *testing.T) {
	defer mockMAL(t, "[]", 0)()

	cfg := malDeleteConfig()
	cfg.AnilistUsernames = nil
	saved := []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 1001}}
	got := fetchListStatuses(cfg, onDiskMediaIDs(saved))

	if _, ok := got["mal:malUser"]; ok {
		t.Fatal("an empty MAL list must leave the account absent")
	}
	got["accountA"] = statuses{1001: "COMPLETED"}
	cfg.AnilistUsernames = []string{"accountA"}
	if deletableMediaIDs(cfg, got, onDiskMediaIDs(saved))[1001] {
		t.Error("with the MAL list unavailable nothing may be deleted")
	}
}

// When only MAL failed, the banner must blame MAL, not AniList.
func TestSearchAnilist_MALOnlyFailureIsItsOwnCause(t *testing.T) {
	defer mockMAL(t, "", http.StatusBadRequest)()

	configs := &files.Config{
		CompletedAnimePath: t.TempDir(),
		MALUsernames:       []string{"nobody"},
		DownloadStatuses:   []string{"CURRENT"},
	}
	_, err := searchAnilist(nil, configs, nil)
	if err == nil {
		t.Fatal("want an error when the only account fails")
	}
	// The abort site wraps it in errCauseAnilist; MAL must still win.
	if code := passErrorCode(fmt.Errorf("%w: %w", errCauseAnilist, err)); code != PassErrMAL {
		t.Errorf("want pass error code %q, got %q", PassErrMAL, code)
	}
}
