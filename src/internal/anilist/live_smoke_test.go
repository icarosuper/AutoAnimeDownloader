package anilist

import (
	"os"
	"testing"
)

// Smoke against the real AniList — the schedule window it returns is why EpisodeList exists
// (see decisions.md #52), and no mock proves it is still that size. Only runs with
// AAD_LIVE_ANILIST=1, so `go test ./...` doesn't depend on the network.
func TestLive_EpisodeList(t *testing.T) {
	if os.Getenv("AAD_LIVE_ANILIST") != "1" {
		t.Skip("set AAD_LIVE_ANILIST=1 to run")
	}
	// 21 = ONE PIECE (clipped schedule), 16498 = Shingeki no Kyojin and 20 = NARUTO (empty schedule).
	for _, id := range []int{21, 16498, 20} {
		ml, err := GetMediaByID(id, PriorityCritical)
		if err != nil || ml == nil {
			t.Fatalf("media %d: %v", id, err)
		}
		eps := EpisodeList(*ml, 1)
		next := 0
		if ml.Media.NextAiringEpisode != nil {
			next = ml.Media.NextAiringEpisode.Episode
		}
		first, last := 0, 0
		if len(eps) > 0 {
			first, last = eps[0].Episode, eps[len(eps)-1].Episode
		}
		t.Logf("media %d: schedule=%d nodes, nextAiring=%d → EpisodeList=%d episodes (%d..%d)",
			id, len(ml.Media.AiringSchedule.Nodes), next, len(eps), first, last)
		if len(eps) == 0 {
			t.Errorf("media %d ended up with no episodes", id)
		}
		if first != 1 {
			t.Errorf("media %d should start at episode 1, started at %d", id, first)
		}
	}
}
