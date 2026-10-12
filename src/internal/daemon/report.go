package daemon

import (
	"AutoAnimeDownloader/src/internal/files"

	"sort"
	"time"
)

// PROBLEM codes: something that should have downloaded and didn't.
const (
	IssueAllAboveSizeLimit = "all_above_size_limit"
	IssueNoSeeders         = "no_seeders"
	IssueNoTorrentFound    = "no_torrent_found"
	IssueDiskFull          = "disk_full"
	IssueTorrentRejected   = "torrent_rejected"
	// IssueAwaitingEarlierEpisode: the episode wasn't searched at all, because an earlier episode
	// of the same anime is still missing (decisions.md #100). Without its own code it fell into
	// no_torrent_found, which claims a search that never happened.
	IssueAwaitingEarlierEpisode = "awaiting_earlier_episode"
)

// LIMIT code: the config working as configured. Different visual weight in the UI because the user
// often doesn't connect the limit with the symptom ("why did it only download 12 of 47?").
const IssueMaxEpisodesPerAnime = "max_episodes_per_anime"

// Values of Issue.BatchSkipped — why max_episodes_per_anime is in force in THAT anime. A skipped
// batch blocks nothing by itself: it only leaves the per-anime limit in force, and the symptom the
// user feels is always the limit. That's why it's a detail field and not a code.
const (
	BatchSkippedNoResult       = "no_result"        // the search returned no pack row at all
	BatchSkippedAboveSizeLimit = "above_size_limit" // there were packs, the filter cut them all
	BatchSkippedNoCoverage     = "no_coverage"      // packs survived, none covers the window
)

// Issue is one row of the report: an (anime, code) pair, with the affected episodes when the code is
// a problem.
//
// Detail fields flattened with omitempty, and not a detail map[string]any: a free map doesn't
// generate usable Swagger nor a decent TS type, and the price is a struct of ten fields with three
// filled at a time.
type Issue struct {
	AnimeID   int    `json:"anime_id" example:"269"`
	AnimeName string `json:"anime_name" example:"Bleach"`
	// Episodes is only filled in a PROBLEM: a problem happens to an episode (it was searched, the
	// candidates were evaluated, that episode didn't download). The limit is the opposite — the
	// daemon stopped considering episodes when it hit the count, so there are no "affected
	// episodes", there's a quantity left over (Downloaded/Pending).
	Episodes []int  `json:"episodes,omitempty"`
	Code     string `json:"code" example:"all_above_size_limit"`

	Candidates int     `json:"candidates,omitempty" example:"8"`
	LimitGB    float64 `json:"limit_gb,omitempty" example:"3"`
	MinSeeders int     `json:"min_seeders,omitempty" example:"1"`
	// Downloaded is how many episodes TOOK a slot under the ceiling, not how many downloaded in this
	// pass: handleAlreadySavedEpisode also increments the counter for an already saved episode.
	// It's the right number for the sentence "downloaded N, M left" — the N are already in the
	// library —, but whoever reads it as "downloads of this pass" will be misled.
	Downloaded   int    `json:"downloaded,omitempty" example:"12"`
	Pending      int    `json:"pending,omitempty" example:"35"`
	BatchSkipped string `json:"batch_skipped,omitempty" example:"no_result"`
}

// CheckReport is the report of the LAST pass, and only of it. It isn't history.
type CheckReport struct {
	FinishedAt time.Time `json:"finished_at" example:"2026-08-19T12:00:00Z"`
	PassError  string    `json:"pass_error" example:""`
	// PassErrorCode is the CAUSE of the abort, for the frontend to build the sentence. PassError keeps
	// carrying the raw text, which the screen shows collapsed: without it a user who wants to open
	// an issue has nothing to paste. See passerror.go.
	PassErrorCode string  `json:"pass_error_code" example:"anilist"`
	Problems      []Issue `json:"problems"`
	Limits        []Issue `json:"limits"`
}

// isLimitCode separates the two categories. Today there's a single limit code; the function exists
// so that adding the second one is one line and not a hunt for scattered comparisons.
func isLimitCode(code string) bool {
	return code == IssueMaxEpisodesPerAnime
}

// aggregateIssues merges the raw Issues of every anime into one per (anime, code) pair and splits
// them into problems and limits, each list sorted by anime name.
//
// ponytail: when two episodes of the same anime hit the same code with different details (ep 12 with
// 8 candidates, ep 13 with 4), the FIRST one's detail fields win and the second's disappear. Summing
// would be a lie ("12 candidates" existed in no search) and listing both would need a detail per
// episode, which is the per-episode report the spec discarded. If that ever bothers, the way is
// Detail []struct{Episode int; ...} inside the Issue.
func aggregateIssues(raw []Issue) (problems, limits []Issue) {
	type key struct {
		animeID int
		code    string
	}
	order := make([]key, 0, len(raw))
	merged := make(map[key]*Issue, len(raw))

	for _, in := range raw {
		k := key{in.AnimeID, in.Code}
		existing, ok := merged[k]
		if !ok {
			cp := in
			cp.Episodes = append([]int(nil), in.Episodes...)
			merged[k] = &cp
			order = append(order, k)
			continue
		}
		existing.Episodes = append(existing.Episodes, in.Episodes...)
	}

	for _, k := range order {
		issue := *merged[k]
		if isLimitCode(issue.Code) {
			limits = append(limits, issue)
			continue
		}
		problems = append(problems, issue)
	}

	sortIssues(problems)
	sortIssues(limits)
	return problems, limits
}

// sortIssues sorts by anime name, with the code as tiebreaker — stable and predictable, without
// inventing a severity ranking nobody asked for.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].AnimeName != issues[j].AnimeName {
			return issues[i].AnimeName < issues[j].AnimeName
		}
		return issues[i].Code < issues[j].Code
	})
}

// searchIssue translates what the per-episode search discarded into the most specific problem code
// that fits.
//
// The order is a CASCADE, not a set: when a filter empties the list, len(magnets) == 0 is also
// true, and "no torrent found" is the least actionable answer of the three — and a lying one,
// because there were eight. The first condition that matches wins, and the order is the business
// rule; same discipline as the deriveAnimeChip cascade in the frontend.
func searchIssue(animeID int, animeName string, episode int, stats dropStats, configs *files.Config) Issue {
	issue := Issue{AnimeID: animeID, AnimeName: animeName, Episodes: []int{episode}}
	switch {
	case stats.Input > 0 && stats.BySize > 0:
		issue.Code = IssueAllAboveSizeLimit
		issue.Candidates = stats.Input
		issue.LimitGB = configs.MaxEpisodeTorrentSizeGB
	case stats.Input > 0 && stats.BySeeders > 0:
		issue.Code = IssueNoSeeders
		issue.Candidates = stats.Input
		issue.MinSeeders = configs.MinSeeders
	default:
		issue.Code = IssueNoTorrentFound
	}
	return issue
}
