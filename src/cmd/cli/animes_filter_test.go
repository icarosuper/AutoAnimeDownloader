package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/api"
	"AutoAnimeDownloader/src/internal/files"
)

func acc(source, user string, watched, pending int) api.AnimeAccount {
	return api.AnimeAccount{Source: source, Username: user, EpisodesWatched: watched, EpisodesPending: pending}
}

// fakeAnimesDaemon answers /config with the given accounts and /animes with the given list.
func fakeAnimesDaemon(t *testing.T, anilist, mal []string, animes []api.AnimeInfo) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch r.URL.Path {
		case "/api/v1/config":
			data = files.Config{AnilistUsernames: anilist, MALUsernames: mal}
		case "/api/v1/animes":
			data = animes
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runAnimesJSON(t *testing.T, srv *httptest.Server, args ...string) []api.AnimeInfo {
	t.Helper()
	stdout, _, err := runCLI(t, append([]string{"--endpoint", srv.URL, "--json", "animes"}, args...)...)
	if err != nil {
		t.Fatalf("animes %v: %v", args, err)
	}
	var got []api.AnimeInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	return got
}

func names(list []api.AnimeInfo) string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Name
	}
	return strings.Join(out, ",")
}

func TestAnimesAccountFilterProjectsProgress(t *testing.T) {
	animes := []api.AnimeInfo{
		{Name: "Both", EpisodesWatched: 2, EpisodesPending: 8, EpisodesReleased: 10, LastDownloadDate: "2026-11-03T00:00:00Z",
			Accounts: []api.AnimeAccount{acc("anilist", "alice", 10, 0), acc("mal", "bob", 2, 8)}},
		{Name: "Bob only", EpisodesWatched: 1, EpisodesReleased: 3, LastDownloadDate: "2026-11-02T00:00:00Z",
			Accounts: []api.AnimeAccount{acc("mal", "bob", 1, 2)}},
		{Name: "Standalone", IsStandalone: true, LastDownloadDate: "2026-11-01T00:00:00Z"},
	}
	srv := fakeAnimesDaemon(t, []string{"alice"}, []string{"bob"}, animes)

	got := runAnimesJSON(t, srv, "--account", "alice")
	if names(got) != "Both" {
		t.Fatalf("alice: got %q, want Both", names(got))
	}
	if got[0].EpisodesWatched != 10 || got[0].EpisodesPending != 0 {
		t.Errorf("alice's progress not projected: watched=%d pending=%d", got[0].EpisodesWatched, got[0].EpisodesPending)
	}
	// "Unwatched" answers for alice, not for bob who is behind on the same anime.
	if got := runAnimesJSON(t, srv, "--account", "alice", "--unwatched"); len(got) != 0 {
		t.Errorf("alice --unwatched: got %q, want none", names(got))
	}

	got = runAnimesJSON(t, srv, "--account", "anilist:alice", "--account", "mal:bob")
	if names(got) != "Both,Bob only" {
		t.Fatalf("alice+bob: got %q", names(got))
	}
	if got[0].EpisodesWatched != 2 || got[0].EpisodesPending != 8 {
		t.Errorf("two accounts: want the one furthest behind, got watched=%d pending=%d", got[0].EpisodesWatched, got[0].EpisodesPending)
	}

	if got := runAnimesJSON(t, srv); len(got) != 3 {
		t.Errorf("no filter: got %q, want all three", names(got))
	}
}

func TestAnimesAccountFilterRejectsUnknownAccount(t *testing.T) {
	srv := fakeAnimesDaemon(t, []string{"alice"}, nil, nil)
	_, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "--account", "carol")
	if err == nil || !strings.Contains(err.Error(), `unknown account "carol"`) || !strings.Contains(err.Error(), "anilist:alice") {
		t.Fatalf("err = %v, want unknown account listing the configured ones", err)
	}
	// The source must match too: alice is only on AniList.
	if _, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "--account", "mal:alice"); err == nil {
		t.Fatal("mal:alice accepted, but alice is only configured on AniList")
	}
}

func TestResolveAccountsBareNameSelectsEveryService(t *testing.T) {
	got, err := resolveAccounts([]string{"alice", "anilist:alice"}, []string{"anilist:alice", "mal:alice", "mal:bob"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "anilist:alice,mal:alice" {
		t.Errorf("got %v", got)
	}
}

func TestAnimesNameTypeAndSort(t *testing.T) {
	animes := []api.AnimeInfo{
		{Name: "Frieren", AltNames: []string{"Sousou no Frieren", "葬送のフリーレン"}, MediaStatus: "FINISHED", EpisodesWatched: 28, LastDownloadDate: "2026-01-01T00:00:00Z"},
		{Name: "Pokémon", MediaStatus: "RELEASING", EpisodesWatched: 3, LastDownloadDate: "2026-11-01T00:00:00Z"},
		{Name: "Apothecary", MediaStatus: "HIATUS", IsBlacklisted: true, EpisodesWatched: 10},
	}
	srv := fakeAnimesDaemon(t, nil, nil, animes)

	cases := []struct {
		args []string
		want string
	}{
		{nil, "Pokémon,Frieren,Apothecary"}, // last-download, most recent first; never downloaded last
		{[]string{"--name", "sousou"}, "Frieren"},
		{[]string{"--name", "フリーレン"}, "Frieren"},
		{[]string{"--name", "pokemon"}, "Pokémon"}, // diacritics stripped
		{[]string{"--type", "airing"}, "Pokémon,Apothecary"},
		{[]string{"--type", "finished"}, "Frieren"},
		{[]string{"--type", "blacklisted"}, "Apothecary"},
		{[]string{"--sort", "name"}, "Apothecary,Frieren,Pokémon"},
		{[]string{"--sort", "name", "--reverse"}, "Pokémon,Frieren,Apothecary"},
		{[]string{"--sort", "watched"}, "Frieren,Apothecary,Pokémon"},
	}
	for _, tc := range cases {
		if got := names(runAnimesJSON(t, srv, tc.args...)); got != tc.want {
			t.Errorf("animes %v = %q, want %q", tc.args, got, tc.want)
		}
	}

	for _, bad := range [][]string{{"--type", "nope"}, {"--sort", "nope"}} {
		if _, _, err := runCLI(t, append([]string{"--endpoint", srv.URL, "animes"}, bad...)...); err == nil {
			t.Errorf("animes %v accepted", bad)
		}
	}
}

func TestAnimesEpisodesPrintsOnListsPanel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: api.AnimeDetailResponse{
			AnimeID:  21,
			Episodes: []api.AnimeEpisodeInfo{{EpisodeNumber: 1}},
			Accounts: []api.AnimeListAccount{{Source: "anilist", Username: "alice", Status: "DROPPED", Progress: 4}},
		}})
	}))
	defer srv.Close()

	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "episodes", "21")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "On lists:") || !strings.Contains(stdout, "alice") || !strings.Contains(stdout, "DROPPED") {
		t.Errorf("missing the on-lists table:\n%s", stdout)
	}
}

func TestAnimesCandidateHitsDetail(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: api.TorrentCandidateDetail{
			NyaaURL: "https://nyaa.si/view/123",
			Files:   []api.TorrentFile{{Name: "ep01.mkv", SizeBytes: 1}},
		}})
	}))
	defer srv.Close()

	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "animes", "candidate", "21", "123")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/animes/21/torrent-candidates/123" {
		t.Errorf("path = %s", path)
	}
	if !strings.Contains(stdout, "https://nyaa.si/view/123") || !strings.Contains(stdout, "ep01.mkv") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

func TestResolveAccountsIgnoresUsernameCase(t *testing.T) {
	configured := []string{"anilist:Alice", "mal:Bob"}
	got, err := resolveAccounts([]string{"alice", "MAL:bob"}, configured)
	if err == nil {
		t.Fatalf("the source is not a username: MAL:bob must not match mal:Bob, got %v", got)
	}
	got, err = resolveAccounts([]string{"alice", "mal:bob"}, configured)
	if err != nil {
		t.Fatal(err)
	}
	// The configured spelling, which is what the daemon stamps on the animes.
	if strings.Join(got, ",") != "anilist:Alice,mal:Bob" {
		t.Errorf("got %v", got)
	}
}

func TestAnimesAccountWhoseListFailed(t *testing.T) {
	animes := []api.AnimeInfo{{Name: "Alice's", Accounts: []api.AnimeAccount{acc("anilist", "alice", 1, 0)}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var data any = animes
		if r.URL.Path == "/api/v1/config" {
			data = files.Config{AnilistUsernames: []string{"alice", "bob"}}
		} else {
			w.Header().Set(api.FailedAccountsHeader, "anilist:bob")
		}
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: data})
	}))
	defer srv.Close()

	// Filtering by the failed account is an error, never an empty list.
	_, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "animes", "--account", "bob")
	if code, exit := cliCode(err); code != "ACCOUNT_LIST_UNAVAILABLE" || exit != 1 {
		t.Fatalf("code=%q exit=%d err=%v, want ACCOUNT_LIST_UNAVAILABLE", code, exit, err)
	}
	// Another account still works; the human output warns on stderr.
	stdout, stderr, err := runCLI(t, "--endpoint", srv.URL, "animes", "--account", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Alice's") || !strings.Contains(stderr, "anilist:bob") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	// --json keeps stderr empty on success.
	if _, stderr, _ := runCLI(t, "--endpoint", srv.URL, "--json", "animes"); stderr != "" {
		t.Errorf("--json success wrote to stderr: %q", stderr)
	}
}

func TestNormalizeNameStripsDiacriticLikeTheDashboard(t *testing.T) {
	// ー (U+30FC) is \p{Diacritic} but not Mn: the dashboard strips it, so must the CLI.
	if got, want := normalizeName("ラーメン"), normalizeName("ラメン"); got != want {
		t.Errorf("normalizeName(ラーメン) = %q, want %q", got, want)
	}
	if got := normalizeName("Pokémon"); got != "pokemon" {
		t.Errorf("normalizeName(Pokémon) = %q", got)
	}
}

func TestSortByNameCollates(t *testing.T) {
	list := []api.AnimeInfo{{Name: "Fate"}, {Name: "Émile"}, {Name: "apple"}}
	sortAnimes(list, "name", false)
	if got := names(list); got != "apple,Émile,Fate" {
		t.Errorf("got %q, want apple,Émile,Fate", got)
	}
}
