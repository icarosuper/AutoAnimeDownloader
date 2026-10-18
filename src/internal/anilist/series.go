package anilist

import (
	"strconv"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// Series is an anime's position on the ABSOLUTE axis of its series — the continuous numbering
// that crosses cours and parts, and that a good share of groups use in file names (SubsPlease
// and Erai number Shingeki Final Season Part 2 as 76..87, not 1..12).
//
// AniList has no franchise id: the PREQUEL chain is the only source, and searching by name is
// a trap (decisions.md #71). Key is the media id of the ROOT of that chain — the series
// identity the schema doesn't offer — and Offset is the sum of the episodes of every ancestor.
//
// The absolute episode of a local episode ep is Offset + ep.
type Series struct {
	// Key is the media id of the oldest anime reachable by PREQUEL. Two animes of the same
	// series share the Key; that's what makes "does the already-downloaded pack cover this
	// episode?" make sense across different anime_ids.
	Key int
	// Offset is how many episodes came before this anime in the series. 0 at the root.
	Offset int
}

// seriesLink is the immutable fact about a media: how many episodes it has and what the media
// id of its prequel is (0 = root). It is only recorded for a node whose position in the
// response was AUTHORITATIVE — see walkSeries's comment on the 2-level cut.
type seriesLink struct {
	PrequelID int
	Episodes  int
}

// seriesCache stores the links by media id. Long TTL because the data is immutable (a finished
// anime doesn't gain an episode or a new prequel) and the walk is monotonic: only ancestors
// go in, and an ancestor of a finished anime is also finished.
//
// It goes into the on-disk snapshot (persist.go), and not to save requests — a whole library
// warm-up costs ~4 requests (decisions.md #71/#72). It's because those 4 requests don't exist
// with AniList down, and without the links the pass loses pack-coverage ownership on the very
// pass that's already working with the rest coming from the cache. Being immutable, it comes
// back from disk with a full TTL: there's nothing to invalidate.
var seriesCache = newTTLCache[seriesLink]()

const seriesTTL = 24 * time.Hour

// seriesNode is a media in the walk response. It's recursive because the query nests
// relations, and Relations only has trustworthy content on the first two levels: on the
// third AniList returns empty edges, which is INDISTINGUISHABLE from "has no prequel"
// (decisions.md #71). Whoever reads needs to know which level they're on — this type
// doesn't carry that information on purpose, so it doesn't look trustworthy everywhere.
type seriesNode struct {
	Id        int         `json:"id"`
	Episodes  *int        `json:"episodes"`
	Format    MediaFormat `json:"format"`
	Status    MediaStatus `json:"status"`
	Relations struct {
		Edges []struct {
			RelationType string     `json:"relationType"`
			Node         seriesNode `json:"node"`
		} `json:"edges"`
	} `json:"relations"`
}

type seriesResponse struct {
	Data struct {
		Page struct {
			Media []seriesNode `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

const seriesNodeFields = `id episodes format status`

// seriesQuery asks for TWO levels of relations, which is the most AniList expands: asking for
// a third would only return empty edges (decisions.md #71). A batch of up to 50 ids costs the
// same budget unit as a single id, so the width is free (decisions.md #72).
var seriesQuery = `
	query GetSeriesChains($ids: [Int]) {
		Page(perPage: ` + strconv.Itoa(anilistMaxPerPage) + `) {
			media(id_in: $ids, type: ANIME) {
				` + seriesNodeFields + `
				relations { edges { relationType node {
					` + seriesNodeFields + `
					relations { edges { relationType node {
						` + seriesNodeFields + `
					} } }
				} } }
			}
		}
	}
`

// GetSeriesIndex resolves the absolute axis of each requested id, walking the PREQUEL chain
// backwards to the root. Returns a map id -> Series; an id ABSENT from the map is an id AniList
// didn't return (deleted, or the search failed before reaching it).
//
// The seed comes ready from the caller: this package can't import files, and the right set
// (animes of the pass plus the anime_ids that already have an episode on disk) only the daemon
// knows.
//
// The error does NOT invalidate the map: what was already resolved before it stays there, for
// the same reason as GetMediaByIDs — a half-answer is better than none, and the caller decides
// what it's worth.
func GetSeriesIndex(ids []int, priority Priority) (map[int]Series, error) {
	known, err := walkSeries(ids, priority)

	result := make(map[int]Series, len(ids))
	for _, id := range ids {
		if series, ok := resolveSeries(known, id); ok {
			result[id] = series
		}
	}
	return result, err
}

// walkSeries is the BFS. Each round consumes a frontier of medias whose chain is not yet
// known, and learns TWO links per fetched media: itself and its prequel, because levels 0
// and 1 of the response come in an authoritative position. The prequel's prequel (level 2)
// has a trustworthy id and episode count, but relations CUT — it goes back to the frontier
// instead of being treated as a root, which is the trap in decisions.md #71.
//
// The loop is depth-agnostic: the cap of 2 is observed behavior, not a contract. It consumes
// whatever comes and enqueues whatever is missing, so an AniList that started expanding 3
// levels would only make the walk finish in fewer rounds.
func walkSeries(seed []int, priority Priority) (map[int]seriesLink, error) {
	known := make(map[int]seriesLink, len(seed))
	seen := make(map[int]bool, len(seed)) // guards against cycles and against fetching the same id twice
	var frontier []int

	var enqueue func(id int)
	enqueue = func(id int) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		if cached, ok := seriesCache.get(seriesKey(id)); ok {
			known[id] = cached
			enqueue(cached.PrequelID) // the chain may continue on nodes not yet in the cache
			return
		}
		frontier = append(frontier, id)
	}

	for _, id := range seed {
		enqueue(id)
	}

	for len(frontier) > 0 {
		batch := frontier
		frontier = nil

		for start := 0; start < len(batch); start += anilistMaxPerPage {
			chunk := batch[start:min(start+anilistMaxPerPage, len(batch))]

			resp, err := sendAnilistRequest[seriesResponse](seriesQuery, RequestVariables{"ids": chunk}, priority)
			if err != nil {
				logger.Logger.Warn().Err(err).Int("media_ids", len(chunk)).
					Msg("Failed to walk a batch of the AniList prequel chain")
				return known, err
			}

			for _, media := range resp.Data.Page.Media {
				// Level 0 and level 1: authoritative relations, the link can be recorded.
				prequel := recordLink(known, media)
				if prequel == nil {
					continue
				}
				seen[prequel.Id] = true
				grandparent := recordLink(known, *prequel)
				if grandparent == nil {
					continue
				}
				// Level 2: we know it exists and how many episodes it has, but not what
				// comes before it. Back to the frontier.
				enqueue(grandparent.Id)
			}
		}
	}

	return known, nil
}

// recordLink records the link of a node whose position in the response was authoritative and
// returns its prequel, or nil if it is a root. Only a FINISHED node with an episode count goes
// into the cache: an airing anime can still gain an episode, and one without a count
// (unreleased) counts as 0 today and as something else tomorrow.
func recordLink(known map[int]seriesLink, node seriesNode) *seriesNode {
	link := seriesLink{}
	if node.Episodes != nil {
		link.Episodes = *node.Episodes
	}

	prequel := prequelOf(node)
	if prequel != nil {
		link.PrequelID = prequel.Id
	}

	known[node.Id] = link
	if node.Status == MediaStatusFinished && node.Episodes != nil {
		seriesCache.set(seriesKey(node.Id), link, seriesTTL)
		markCacheDirty()
	}
	return prequel
}

// prequelOf returns the TV/TV_SHORT prequel of the node. Recap movies, OVAs and ONAs also
// arrive as PREQUEL with Episodes filled, but they don't enter the count the groups follow —
// same rule as daemon.ComputeEpisodeOffset (decisions.md #9 and #71).
//
// The selection rule is the same as daemon.ComputeEpisodeOffset, and the two stay separate on
// purpose: here it walks the WHOLE chain (absolute series offset), there it takes ONE hop
// (total of the immediate prequel). Those are the two distinct pack-numbering hypotheses in
// daemon.packAxis, so unifying them would erase the distinction (decisions.md #79).
func prequelOf(node seriesNode) *seriesNode {
	for i := range node.Relations.Edges {
		edge := &node.Relations.Edges[i]
		if edge.RelationType != "PREQUEL" {
			continue
		}
		if edge.Node.Format != MediaFormatTV && edge.Node.Format != MediaFormatTVShort {
			continue
		}
		return &edge.Node
	}
	return nil
}

// resolveSeries sums the chain backwards from id. It stops at the first unknown ancestor:
// when in doubt the walk resolves to the smallest possible offset, because a missing offset
// falls back to relative numbering (which a good share of groups use) and a wrong offset
// sends the search to an episode that doesn't exist (decisions.md #71).
func resolveSeries(known map[int]seriesLink, id int) (Series, bool) {
	link, ok := known[id]
	if !ok {
		return Series{}, false
	}

	series := Series{Key: id}
	visited := map[int]bool{id: true}
	for parent := link.PrequelID; parent != 0; {
		ancestor, ok := known[parent]
		if !ok || visited[parent] {
			break
		}
		visited[parent] = true
		series.Offset += ancestor.Episodes
		series.Key = parent
		parent = ancestor.PrequelID
	}
	return series, true
}
