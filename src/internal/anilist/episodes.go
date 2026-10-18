package anilist

import "sort"

// EpisodeList returns the episodes of an anime from fromEpisode onward, and is the ONLY source of
// "which episodes exist" in the app.
//
// Raw media.airingSchedule can't be used for this: AniList keeps a WINDOW of ~500 schedule
// entries per media and discards the old ones. On One Piece page 1 starts at episode
// 1123 — the previous 1122 do not exist in the API — and on an old/finished anime (Naruto,
// Shingeki no Kyojin) the schedule comes back COMPLETELY empty. While the episode list came
// from there, those animes simply had no episodes to download (see decisions.md #52).
//
// That's why the list is SYNTHESIZED from fromEpisode to the last aired episode, using the real
// schedule node when it exists (it brings a real airingAt/timeUntilAiring) and a fabricated
// node when it doesn't. Upcoming episodes come only from the schedule, which is who knows
// their date.
//
// A fabricated node has ID zero: after the key migration to (AnimeID, EpisodeNumber) nothing
// identifies an episode by schedule ID, so it doesn't need to be unique (see decisions.md #52).
func EpisodeList(ml MediaList, fromEpisode int) []AiringNode {
	if fromEpisode < 1 {
		fromEpisode = 1
	}

	lastAired := lastAiredEpisode(ml)

	byEpisode := make(map[int]AiringNode, len(ml.Media.AiringSchedule.Nodes))
	for _, n := range ml.Media.AiringSchedule.Nodes {
		byEpisode[n.Episode] = n
	}

	result := make([]AiringNode, 0, max(lastAired-fromEpisode+1, 0)+len(ml.Media.AiringSchedule.Nodes))
	for ep := fromEpisode; ep <= lastAired; ep++ {
		if node, ok := byEpisode[ep]; ok {
			result = append(result, node)
			continue
		}
		// Negative timeUntilAiring = already aired, which is the only thing the rest of the
		// code asks of a past episode. airingAt stays zero: AniList no longer keeps the date.
		result = append(result, AiringNode{Episode: ep, TimeUntilAiring: -1})
	}

	// Upcoming ones: the schedule, plus nextAiringEpisode. It has to go in explicitly because
	// on a clipped-window anime page 1 of the schedule only has PAST episodes (on One Piece it
	// goes from 1123 to 1147, and the next is 1174) — without this the screen would show the
	// anime with no upcoming episode.
	future := append([]AiringNode{}, ml.Media.AiringSchedule.Nodes...)
	if next := ml.Media.NextAiringEpisode; next != nil {
		future = append(future, *next)
	}
	seen := make(map[int]bool, len(future))
	for _, n := range future {
		if n.Episode > lastAired && n.Episode >= fromEpisode && !seen[n.Episode] {
			seen[n.Episode] = true
			result = append(result, n)
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Episode < result[j].Episode })
	return result
}

// lastAiredEpisode is the highest episode number that has already aired, combining the three
// sources AniList offers — none of them reliable on its own: the schedule may be clipped or
// empty, nextAiringEpisode is nil on a finished anime, and media.episodes only holds when the
// anime has finished (on a RELEASING it is the PREDICTED count of the whole season, not what
// has already aired).
// LastAiredEpisode is the exported version of lastAiredEpisode. It serves as a "series size"
// measure outside the package — media.Episodes doesn't work for that: on a RELEASING with no
// announced end (One Piece) it is nil, and that's exactly the long series that needs to be
// recognized as such.
func LastAiredEpisode(ml MediaList) int {
	return lastAiredEpisode(ml)
}

func lastAiredEpisode(ml MediaList) int {
	last := 0
	for _, n := range ml.Media.AiringSchedule.Nodes {
		if n.TimeUntilAiring <= 0 && n.Episode > last {
			last = n.Episode
		}
	}
	if next := ml.Media.NextAiringEpisode; next != nil && next.Episode-1 > last {
		last = next.Episode - 1
	}
	if ml.Media.Status == MediaStatusFinished && ml.Media.Episodes != nil && *ml.Media.Episodes > last {
		last = *ml.Media.Episodes
	}
	return last
}
