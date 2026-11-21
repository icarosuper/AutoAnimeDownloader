package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"AutoAnimeDownloader/src/internal/api"

	"golang.org/x/text/unicode/norm"
)

// The `animes` filters are a Go port of the dashboard's, which are client-side
// (frontend/src/lib/utils/status.ts: scopeToAccounts, filterAnimes, sortAnimes). /api/v1/animes
// takes no filter on purpose (decisions.md #125), so the CLI filters what it gets the same way
// the screen does. Change one, change the other.

type animesView struct {
	accounts  []string // resolved account keys, "<source>:<username>"; empty = no user filter
	name      string
	unwatched bool
	typ       string
	sortKey   string
	reverse   bool
}

var animeTypes = []string{"all", "airing", "finished", "standalone", "blacklisted"}

// animeSortKeys maps the CLI spelling to the dashboard's SortKey.
var animeSortKeys = map[string]string{
	"name":          "name",
	"watched":       "episodes_watched",
	"last-download": "last_download_date",
}

func accountKey(source, username string) string { return source + ":" + username }

// configuredAccounts lists every configured account key, AniList first, in config order — the
// options the dashboard user filter offers.
func configuredAccounts(anilist, mal []string) []string {
	keys := make([]string, 0, len(anilist)+len(mal))
	for _, u := range anilist {
		keys = append(keys, accountKey("anilist", u))
	}
	for _, u := range mal {
		keys = append(keys, accountKey("mal", u))
	}
	return keys
}

// resolveAccounts turns --account values into account keys. A value is either a full key
// (`anilist:alice`, `mal:alice`) or a bare username, which selects that name on every service
// it is configured on. An account the config does not have is an error, never an empty list:
// a typo must not pass for "this account has nothing".
func resolveAccounts(values, configured []string) ([]string, error) {
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		var matched []string
		for _, key := range configured {
			_, user, _ := strings.Cut(key, ":")
			if key == v || user == v {
				matched = append(matched, key)
			}
		}
		if len(matched) == 0 {
			if len(configured) == 0 {
				return nil, fmt.Errorf("unknown account %q: no AniList or MAL account is configured", v)
			}
			return nil, fmt.Errorf("unknown account %q; configured: %s", v, strings.Join(configured, ", "))
		}
		for _, key := range matched {
			if !slices.Contains(out, key) {
				out = append(out, key)
			}
		}
	}
	return out, nil
}

// scopeToAccounts keeps the animes at least one selected account tracks (OR) and re-projects
// episodes_watched / episodes_pending onto THOSE accounts: the lowest progress, the highest
// pending (decisions.md #125). An empty selection returns the list untouched.
func scopeToAccounts(animes []api.AnimeInfo, selected []string) []api.AnimeInfo {
	if len(selected) == 0 {
		return animes
	}
	scoped := make([]api.AnimeInfo, 0, len(animes))
	for _, a := range animes {
		found := false
		for _, acc := range a.Accounts {
			if !slices.Contains(selected, accountKey(acc.Source, acc.Username)) {
				continue
			}
			if !found {
				a.EpisodesWatched, a.EpisodesPending = acc.EpisodesWatched, acc.EpisodesPending
				found = true
				continue
			}
			a.EpisodesWatched = min(a.EpisodesWatched, acc.EpisodesWatched)
			a.EpisodesPending = max(a.EpisodesPending, acc.EpisodesPending)
		}
		if found {
			scoped = append(scoped, a)
		}
	}
	return scoped
}

// normalizeName is the dashboard search's NFKD + diacritic strip + lowercase, so "kimi no na wa"
// matches "Kimi no Na wa" and full-width katakana matches the normal form.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func matchesAnimeType(a api.AnimeInfo, typ string) bool {
	switch typ {
	case "airing":
		return a.MediaStatus == "RELEASING" || a.MediaStatus == "HIATUS"
	case "finished":
		return a.MediaStatus == "FINISHED" || a.MediaStatus == "CANCELLED"
	case "standalone":
		return a.IsStandalone
	case "blacklisted":
		return a.IsBlacklisted
	default:
		return true
	}
}

// filterAnimes is the dashboard's search + "Unwatched" + type filter. The search matches ANY of
// the anime's titles (alt_names carries romaji, kanji and synonyms).
func filterAnimes(animes []api.AnimeInfo, v animesView) []api.AnimeInfo {
	term := normalizeName(strings.TrimSpace(v.name))
	out := make([]api.AnimeInfo, 0, len(animes))
	for _, a := range animes {
		if term != "" && !slices.ContainsFunc(append([]string{a.Name}, a.AltNames...), func(n string) bool {
			return strings.Contains(normalizeName(n), term)
		}) {
			continue
		}
		if v.unwatched && a.EpisodesWatched >= a.EpisodesReleased && a.EpisodesReleased > 0 {
			continue
		}
		if !matchesAnimeType(a, v.typ) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// sortAnimes is the dashboard's sort: name ascending, the other keys descending, --reverse flips.
// The API returns the animes in map order, so without a sort the output order is random.
func sortAnimes(animes []api.AnimeInfo, key string, reverse bool) {
	desc := key != "name"
	if reverse {
		desc = !desc
	}
	lastDownload := func(a api.AnimeInfo) int64 {
		t, err := time.Parse(time.RFC3339, a.LastDownloadDate)
		if err != nil {
			return 0
		}
		return t.UnixNano()
	}
	sort.SliceStable(animes, func(i, j int) bool {
		var less, greater bool
		switch key {
		case "name":
			c := strings.Compare(strings.ToLower(animes[i].Name), strings.ToLower(animes[j].Name))
			less, greater = c < 0, c > 0
		case "episodes_watched":
			less, greater = animes[i].EpisodesWatched < animes[j].EpisodesWatched, animes[i].EpisodesWatched > animes[j].EpisodesWatched
		default:
			li, lj := lastDownload(animes[i]), lastDownload(animes[j])
			less, greater = li < lj, li > lj
		}
		if desc {
			return greater
		}
		return less
	})
}
