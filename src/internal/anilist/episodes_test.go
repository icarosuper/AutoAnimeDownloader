package anilist

import "testing"

func mediaList(status MediaStatus, totalEpisodes *int, next *int, scheduled ...AiringNode) MediaList {
	ml := MediaList{Media: Media{Id: 21, Status: status, Episodes: totalEpisodes}}
	ml.Media.AiringSchedule.Nodes = scheduled
	if next != nil {
		ml.Media.NextAiringEpisode = &AiringNode{Episode: *next, TimeUntilAiring: 3600}
	}
	return ml
}

func numbers(nodes []AiringNode) []int {
	out := make([]int, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Episode)
	}
	return out
}

// One Piece's schedule starts at 1123 (AniList discards old entries), and nextAiringEpisode
// is the only hint of what has already aired. Without synthesizing, the first 1122 wouldn't exist.
func TestEpisodeList_FillsGapBelowTheAniListWindow(t *testing.T) {
	next := 1174
	ml := mediaList(MediaStatusReleasing, nil, &next,
		AiringNode{ID: 416348, Episode: 1123, TimeUntilAiring: -100, AiringAt: 42},
		AiringNode{ID: 416349, Episode: 1124, TimeUntilAiring: -50},
		AiringNode{ID: 416400, Episode: 1174, TimeUntilAiring: 3600},
	)

	got := EpisodeList(ml, 1)
	if len(got) != 1174 {
		t.Fatalf("want 1174 episodes (1..1173 aired + 1174 upcoming), got %d", len(got))
	}
	if got[0].Episode != 1 || got[0].TimeUntilAiring >= 0 {
		t.Errorf("episode 1 must be synthesized as already aired, got %+v", got[0])
	}
	if got[1122].Episode != 1123 || got[1122].AiringAt != 42 {
		t.Errorf("where the schedule exists the REAL node must be used (it brings airingAt), got %+v", got[1122])
	}
	if last := got[1173]; last.Episode != 1174 || last.TimeUntilAiring <= 0 {
		t.Errorf("the upcoming episode must enter as future, got %+v", last)
	}
}

// An old/finished anime comes back with an EMPTY airingSchedule: the list has to come from
// media.episodes, otherwise none of it is downloadable.
func TestEpisodeList_FinishedAnimeWithEmptySchedule(t *testing.T) {
	total := 25
	got := EpisodeList(mediaList(MediaStatusFinished, &total, nil), 1)
	if len(got) != 25 {
		t.Fatalf("want 25 episodes, got %d: %v", len(got), numbers(got))
	}
	for i, n := range got {
		if n.Episode != i+1 || n.TimeUntilAiring > 0 {
			t.Fatalf("unexpected episode %d: %+v", i+1, n)
		}
	}
}

// fromEpisode is progress+1: what the user already watched doesn't enter the list.
func TestEpisodeList_StartsAtFromEpisode(t *testing.T) {
	total := 12
	got := numbers(EpisodeList(mediaList(MediaStatusFinished, &total, nil), 10))
	want := []int{10, 11, 12}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

// On a RELEASING, media.episodes is the PREDICTED season count: using it as "last aired"
// would send the daemon looking for an episode that doesn't exist yet. nextAiringEpisode
// enters the list, but as FUTURE (the daemon skips it, the screen shows it).
func TestEpisodeList_ReleasingIgnoresPredictedTotal(t *testing.T) {
	total := 24
	next := 6
	got := EpisodeList(mediaList(MediaStatusReleasing, &total, &next), 1)
	if len(got) != 6 {
		t.Fatalf("want 5 aired + the next one, got %v", numbers(got))
	}
	for _, n := range got[:5] {
		if n.TimeUntilAiring > 0 {
			t.Fatalf("episode %d should have aired: %+v", n.Episode, n)
		}
	}
	if got[5].Episode != 6 || got[5].TimeUntilAiring <= 0 {
		t.Fatalf("the next episode must enter as future, got %+v", got[5])
	}
}

// NOT_YET_RELEASED has no episode on air: the list brings only what the schedule promises.
func TestEpisodeList_NotYetReleased(t *testing.T) {
	got := EpisodeList(mediaList(MediaStatusNotYetReleased, nil, nil,
		AiringNode{Episode: 1, TimeUntilAiring: 7200},
	), 1)
	if len(got) != 1 || got[0].Episode != 1 || got[0].TimeUntilAiring <= 0 {
		t.Fatalf("want only upcoming episode 1, got %+v", got)
	}
}
