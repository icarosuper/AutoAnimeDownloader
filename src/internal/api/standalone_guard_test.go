package api

import "testing"

// TestBlockReason covers the whole table. The same blockReason serves the POST's 409 and the
// search result's block_reason, on purpose: "the front won't let you click" and "the back
// returns an error" have to agree by construction, not by discipline.
func TestBlockReason(t *testing.T) {
	cases := []struct {
		name          string
		guard         standaloneGuard
		mediaID       int
		totalEpisodes int
		want          string
	}{
		{
			name:  "nothing blocks",
			guard: standaloneGuard{},
			want:  "",
		},
		{
			name:    "already standalone",
			guard:   standaloneGuard{standalone: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonStandalone,
		},
		{
			name:    "on the processed list",
			guard:   standaloneGuard{tracked: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonTracked,
		},
		{
			name:    "on an excluded list",
			guard:   standaloneGuard{blacklisted: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonBlacklist,
		},
		{
			name:          "24 records of 24 episodes",
			guard:         standaloneGuard{downloaded: map[int]int{21: 24}},
			mediaID:       21,
			totalEpisodes: 24,
			want:          blockReasonDownloaded,
		},
		{
			// 12 is the per-anime limit, not "already downloaded" — the case the batch<->limit
			// rule starts to complete.
			name:          "12 records of 24 episodes is not already downloaded",
			guard:         standaloneGuard{downloaded: map[int]int{21: 12}},
			mediaID:       21,
			totalEpisodes: 24,
			want:          "",
		},
		{
			name:          "unknown total never blocks",
			guard:         standaloneGuard{downloaded: map[int]int{21: 300}},
			mediaID:       21,
			totalEpisodes: 0,
			want:          "",
		},
		{
			// Precedence matters: blacklist is the only reason where adding would change
			// behavior for the worse (the standalone record bypasses the user's filter).
			name: "standalone AND on an excluded list returns blacklist",
			guard: standaloneGuard{
				standalone:  map[int]bool{21: true},
				blacklisted: map[int]bool{21: true},
			},
			mediaID: 21,
			want:    blockReasonBlacklist,
		},
		{
			name: "standalone AND on the list returns standalone",
			guard: standaloneGuard{
				standalone: map[int]bool{21: true},
				tracked:    map[int]bool{21: true},
			},
			mediaID: 21,
			want:    blockReasonStandalone,
		},
		{
			name: "on the list AND already downloaded returns tracked",
			guard: standaloneGuard{
				tracked:    map[int]bool{21: true},
				downloaded: map[int]int{21: 24},
			},
			mediaID:       21,
			totalEpisodes: 24,
			want:          blockReasonTracked,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.guard.blockReason(tc.mediaID, tc.totalEpisodes); got != tc.want {
				t.Fatalf("blockReason(%d, %d) = %q, want %q", tc.mediaID, tc.totalEpisodes, got, tc.want)
			}
		})
	}
}

// TestBlockReason_PlanningIsNotTracked: a PLANNING anime with download_statuses=[CURRENT] is
// on an AniList list and the daemon ignores it — adding it as standalone is the most obvious
// use of the feature, not an error to block. If this test becomes "tracked", the definition of
// "tracked" regressed to "there is an AniList entry".
func TestBlockReason_PlanningIsNotTracked(t *testing.T) {
	// tracked comes from the snapshot the daemon PROCESSES (fetchAniListEntries, already
	// filtered by download_statuses), so the PLANNING entry simply is not there.
	guard := standaloneGuard{tracked: map[int]bool{}}

	if got := guard.blockReason(21, 12); got != "" {
		t.Fatalf("a PLANNING anime outside download_statuses must be addable, got %q", got)
	}
}
