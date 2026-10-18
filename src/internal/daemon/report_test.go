package daemon

import (
	"AutoAnimeDownloader/src/internal/files"

	"errors"
	"reflect"
	"testing"
	"time"
)

// TestAggregateIssues: one Issue per (anime, code) pair. Two episodes of the same anime with
// the same code become ONE entry with two numbers; different codes become two.
func TestAggregateIssues(t *testing.T) {
	raw := []Issue{
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{12}, Code: IssueAllAboveSizeLimit, Candidates: 8, LimitGB: 3},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{15}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{13}, Code: IssueAllAboveSizeLimit, Candidates: 4, LimitGB: 3},
		{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35, BatchSkipped: BatchSkippedNoResult},
	}

	problems, limits := aggregateIssues(raw)

	if len(limits) != 1 {
		t.Fatalf("want 1 limit, got %d (%+v)", len(limits), limits)
	}
	if limits[0].Code != IssueMaxEpisodesPerAnime || limits[0].Downloaded != 12 || limits[0].Pending != 35 {
		t.Errorf("wrong limit: %+v", limits[0])
	}
	// A limit does not carry episode numbers: the daemon stopped CONSIDERING episodes when it
	// hit the count, so there are no "affected episodes".
	if len(limits[0].Episodes) != 0 {
		t.Errorf("a limit must not carry Episodes, got %+v", limits[0].Episodes)
	}

	if len(problems) != 2 {
		t.Fatalf("want 2 problems, got %d (%+v)", len(problems), problems)
	}
	byCode := map[string]Issue{}
	for _, p := range problems {
		byCode[p.Code] = p
	}
	size, ok := byCode[IssueAllAboveSizeLimit]
	if !ok {
		t.Fatalf("missing the size problem in %+v", problems)
	}
	if !reflect.DeepEqual(size.Episodes, []int{12, 13}) {
		t.Errorf("want episodes [12 13], got %+v", size.Episodes)
	}
	if size.Candidates != 8 || size.LimitGB != 3 {
		t.Errorf("the first episode's detail must win, got %+v", size)
	}
	if _, ok := byCode[IssueNoTorrentFound]; !ok {
		t.Errorf("missing the no-torrent-found problem in %+v", problems)
	}
}

// TestAggregateIssues_SortedByAnimeName: stable, predictable order, no severity ranking.
func TestAggregateIssues_SortedByAnimeName(t *testing.T) {
	raw := []Issue{
		{AnimeID: 3, AnimeName: "Zeta", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 1, AnimeName: "Alpha", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Mu", Episodes: []int{1}, Code: IssueNoTorrentFound},
	}
	problems, _ := aggregateIssues(raw)
	want := []string{"Alpha", "Mu", "Zeta"}
	for i, name := range want {
		if problems[i].AnimeName != name {
			t.Errorf("position %d: want %q, got %q", i, name, problems[i].AnimeName)
		}
	}
}

func TestAggregateIssues_Empty(t *testing.T) {
	problems, limits := aggregateIssues(nil)
	if problems != nil || limits != nil {
		t.Errorf("a clean pass must produce nil/nil, got %+v / %+v", problems, limits)
	}
}

// TestSetLastCheckError_ClearsReport: a pass that aborted before looking at any anime has no
// per-anime report — it has pass_error. Without the clear, the screen would show the PREVIOUS
// pass's problems side by side with a new pass error.
func TestSetLastCheckError_ClearsReport(t *testing.T) {
	s := NewState()
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueNoTorrentFound}},
	})
	if len(s.GetLastCheckReport().Problems) != 1 {
		t.Fatal("the report should have been stored before the error")
	}

	s.SetLastCheckError(errors.New("anilist is down"))

	got := s.GetLastCheckReport()
	if len(got.Problems) != 0 || len(got.Limits) != 0 || !got.FinishedAt.IsZero() {
		t.Errorf("SetLastCheckError must clear the report, got %+v", got)
	}
}

// TestSetLastCheckReport_AfterClearingError: SetLastCheckError(nil) also clears, so the
// SetLastCheckReport at the end of the pass MUST come after it — and survive.
func TestSetLastCheckReport_AfterClearingError(t *testing.T) {
	s := NewState()
	s.SetLastCheckError(nil)
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Limits:     []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35}},
	})
	if len(s.GetLastCheckReport().Limits) != 1 {
		t.Error("the report published after SetLastCheckError(nil) must survive")
	}
}

// TestSearchIssue_CascadePrecedence: the first three problems overlap — when a filter empties
// the list, "no torrent found" is also true. The specific MUST beat the generic: "every
// candidate was 8 GB and your ceiling is 3 GB" is actionable; "no torrent found" is not, and
// on top of that it is a lie — there were eight.
func TestSearchIssue_CascadePrecedence(t *testing.T) {
	configs := &files.Config{MaxEpisodeTorrentSizeGB: 3, MinSeeders: 5}

	t.Run("cut by size beats the generic", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 8}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Fatalf("want %q, got %q", IssueAllAboveSizeLimit, got.Code)
		}
		if got.Candidates != 8 || got.LimitGB != 3 {
			t.Errorf("wrong details: %+v", got)
		}
		if len(got.Episodes) != 1 || got.Episodes[0] != 12 {
			t.Errorf("want episode [12], got %+v", got.Episodes)
		}
	})

	t.Run("size beats seeders when both cut", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 5, BySeeders: 3}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Errorf("want %q, got %q", IssueAllAboveSizeLimit, got.Code)
		}
	})

	t.Run("cut only by seeders", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 4, BySeeders: 4}, configs)
		if got.Code != IssueNoSeeders {
			t.Fatalf("want %q, got %q", IssueNoSeeders, got.Code)
		}
		if got.Candidates != 4 || got.MinSeeders != 5 {
			t.Errorf("wrong details: %+v", got)
		}
	})

	t.Run("search really empty", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{}, configs)
		if got.Code != IssueNoTorrentFound {
			t.Errorf("want %q, got %q", IssueNoTorrentFound, got.Code)
		}
		if got.Candidates != 0 || got.LimitGB != 0 || got.MinSeeders != 0 {
			t.Errorf("no_torrent_found has no detail, got %+v", got)
		}
	})
}
