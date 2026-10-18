package anilist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// fixtureMedia is a node of the fake AniList graph used by the walk tests.
type fixtureMedia struct {
	episodes *int
	format   MediaFormat
	status   MediaStatus
	prequel  int
	sequel   int
}

func eps(n int) *int { return &n }

// tvChain builds a finished TV chain from (id, episodes) pairs, oldest to newest, linking
// PREQUEL/SEQUEL both ways — as AniList returns.
func tvChain(graph map[int]*fixtureMedia, ids []int, counts []int) {
	for i, id := range ids {
		node := &fixtureMedia{episodes: eps(counts[i]), format: MediaFormatTV, status: MediaStatusFinished}
		if i > 0 {
			node.prequel = ids[i-1]
			graph[ids[i-1]].sequel = id
		}
		graph[id] = node
	}
}

// mockSeriesAPI answers the walk query from the graph, rendering EXACTLY two levels of
// relations and returning `edges: []` on the third — which is what AniList actually does
// (decisions.md #71). That's the part of the mock that matters: a fake that expanded the
// whole depth would let through exactly the bug of treating the cut level as a root.
func mockSeriesAPI(t *testing.T, graph map[int]*fixtureMedia) *int {
	t.Helper()
	requests := 0
	restore := MockAniListDo(func(req *http.Request) (*http.Response, error) {
		requests++

		body, _ := io.ReadAll(req.Body)
		var parsed struct {
			Variables struct {
				Ids []int `json:"ids"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Errorf("unreadable request body: %v", err)
		}
		if len(parsed.Variables.Ids) > anilistMaxPerPage {
			t.Errorf("batch of %d ids exceeds AniList's max perPage", len(parsed.Variables.Ids))
		}

		rendered := make([]string, 0, len(parsed.Variables.Ids))
		for _, id := range parsed.Variables.Ids {
			if node, ok := graph[id]; ok {
				rendered = append(rendered, renderMedia(graph, id, node, 0))
			}
		}
		return respond(200, `{"data":{"Page":{"media":[`+strings.Join(rendered, ",")+`]}}}`, nil), nil
	})
	t.Cleanup(restore)
	return &requests
}

func renderMedia(graph map[int]*fixtureMedia, id int, node *fixtureMedia, depth int) string {
	episodes := "null"
	if node.episodes != nil {
		episodes = strconv.Itoa(*node.episodes)
	}

	edges := []string{}
	if depth < 2 { // on the third level AniList stops expanding
		for _, rel := range []struct {
			kind string
			id   int
		}{{"PREQUEL", node.prequel}, {"SEQUEL", node.sequel}} {
			if child, ok := graph[rel.id]; ok {
				edges = append(edges, fmt.Sprintf(`{"relationType":%q,"node":%s}`,
					rel.kind, renderMedia(graph, rel.id, child, depth+1)))
			}
		}
	}

	return fmt.Sprintf(`{"id":%d,"episodes":%s,"format":%q,"status":%q,"relations":{"edges":[%s]}}`,
		id, episodes, node.format, node.status, strings.Join(edges, ","))
}

// goldenChains are the three chains measured against the real AniList on 28/Aug/2026
// (decisions.md #71). They live here, not in a markdown, because markdown rots in silence
// and a test breaks CI.
//
// Shingeki is the strong validation: `76..87` for Final Season Part 2 is EXACTLY the numbering
// SubsPlease and Erai-raws use in file names. The chain reproduces the real convention with
// no adjustment.
var goldenChains = []struct {
	root   int
	ids    []int
	counts []int
	first  []int // first absolute episode of each id
}{
	{
		root:   108465,
		ids:    []int{108465, 127720, 146065, 166873},
		counts: []int{11, 12, 13, 12},
		first:  []int{1, 12, 24, 37},
	},
	{
		root:   16498,
		ids:    []int{16498, 20958, 99147, 104578, 110277, 131681},
		counts: []int{25, 12, 12, 10, 16, 12},
		first:  []int{1, 26, 38, 50, 60, 76},
	},
	{
		root:   15689,
		ids:    []int{15689, 5081, 11597, 17074, 21262, 20918, 21745},
		counts: []int{4, 15, 11, 26, 12, 4, 7},
		first:  []int{1, 5, 20, 31, 57, 69, 73},
	},
}

func goldenGraph() map[int]*fixtureMedia {
	graph := map[int]*fixtureMedia{}
	for _, chain := range goldenChains {
		tvChain(graph, chain.ids, chain.counts)
	}
	return graph
}

// TestGetSeriesIndex_GoldenChains is the real validation of the absolute axis: the three
// measured chains, resolved end to end. Two of them are taller than the 2 levels a single
// AniList query expands, so passing here proves the walk continues after the cut instead of
// treating `edges: []` as a root.
func TestGetSeriesIndex_GoldenChains(t *testing.T) {
	graph := goldenGraph()
	requests := mockSeriesAPI(t, graph)

	// The seed is ONLY the tips — the cour someone is watching. Seeding the whole chain
	// would let every ancestor arrive as level 0 of the response, and the 2-level cut
	// would never be exercised.
	var tips []int
	for _, chain := range goldenChains {
		tips = append(tips, chain.ids[len(chain.ids)-1])
	}

	index, err := GetSeriesIndex(tips, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	for _, chain := range goldenChains {
		checkGolden(t, index, chain.ids[len(chain.ids)-1], chain.root, chain.first[len(chain.ids)-1], chain.counts[len(chain.counts)-1])
	}

	// The tallest chain has 7 levels and each query walks 2, so 3 rounds cover the three
	// chains together. The number is the point of the feature: 17 medias in a few requests.
	// An implementation that fetched one ancestor per request would pass everything above
	// and blow AniList's budget in production.
	if *requests > 4 {
		t.Errorf("want at most 4 requests for the three chains, got %d", *requests)
	}

	// Now the whole chain, so the middle positions get locked too. Comes from the cache.
	var all []int
	for _, chain := range goldenChains {
		all = append(all, chain.ids...)
	}
	index, err = GetSeriesIndex(all, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex (whole chain): %v", err)
	}
	for _, chain := range goldenChains {
		for i, id := range chain.ids {
			checkGolden(t, index, id, chain.root, chain.first[i], chain.counts[i])
		}
	}
}

func checkGolden(t *testing.T, index map[int]Series, id, wantKey, wantFirst, count int) {
	t.Helper()
	series, ok := index[id]
	if !ok {
		t.Fatalf("media %d was left out of the index", id)
	}
	if series.Key != wantKey {
		t.Errorf("media %d: want seriesKey %d, got %d", id, wantKey, series.Key)
	}
	if got, want := series.Offset+1, wantFirst; got != want {
		t.Errorf("media %d: want abs %d..%d, got %d..%d", id, want, want+count-1, got, got+count-1)
	}
}

// TestGetSeriesIndex_CacheAvoidsSecondWalk: the data is immutable and the walk is monotonic,
// so the second read can't go to the network. That's what makes the per-pass warm-up cheap.
func TestGetSeriesIndex_CacheAvoidsSecondWalk(t *testing.T) {
	requests := mockSeriesAPI(t, goldenGraph())

	if _, err := GetSeriesIndex([]int{131681}, PriorityDisposable); err != nil {
		t.Fatalf("first walk: %v", err)
	}
	before := *requests

	index, err := GetSeriesIndex([]int{131681}, PriorityDisposable)
	if err != nil {
		t.Fatalf("second walk: %v", err)
	}
	if *requests != before {
		t.Errorf("the second walk went to the network %d times; the 24h cache should cover everything", *requests-before)
	}
	if index[131681].Offset != 75 {
		t.Errorf("the cache response has to be the same: want offset 75, got %d", index[131681].Offset)
	}
}

// TestGetSeriesIndex_UnreleasedCountsZeroAndIsNotCached: an anime without an episode count
// counts as 0 today and as something else tomorrow. Persisting that would freeze the zero
// for 24h and shift the whole series down.
func TestGetSeriesIndex_UnreleasedCountsZeroAndIsNotCached(t *testing.T) {
	graph := map[int]*fixtureMedia{}
	tvChain(graph, []int{100, 200}, []int{12, 12})
	graph[100].episodes = nil
	graph[100].status = MediaStatusNotYetReleased

	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{200}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[200].Offset != 0 {
		t.Errorf("a prequel without a count has to add 0, got offset %d", index[200].Offset)
	}
	if _, cached := seriesCache.get("100"); cached {
		t.Error("a prequel without an episode count can't enter the 24h cache")
	}
	if _, cached := seriesCache.get("200"); !cached {
		t.Error("the finished anime with a count should have been cached")
	}
}

// TestGetSeriesIndex_SkipsNonTVPrequel: recap movies, OVAs and ONAs also arrive as PREQUEL
// with episodes filled, and summing that sends the search to an episode that doesn't exist.
// Same rule as ComputeEpisodeOffset (decisions.md #9 and #71).
func TestGetSeriesIndex_SkipsNonTVPrequel(t *testing.T) {
	graph := map[int]*fixtureMedia{
		300: {episodes: eps(4), format: MediaFormatOVA, status: MediaStatusFinished, sequel: 400},
		400: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 300},
	}
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{400}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[400].Offset != 0 || index[400].Key != 400 {
		t.Errorf("an OVA prequel can't enter the chain: got %+v", index[400])
	}
}

// TestGetSeriesIndex_MissingMediaIsAbsent: an id AniList doesn't return doesn't become offset
// 0 in the map — it becomes absence. Mixing the two would make an anime deleted from AniList
// look like a series root, with silently wrong relative numbering.
func TestGetSeriesIndex_MissingMediaIsAbsent(t *testing.T) {
	graph := map[int]*fixtureMedia{}
	tvChain(graph, []int{500}, []int{12})
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{500, 999}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if _, ok := index[999]; ok {
		t.Error("an id unknown to AniList has to stay ABSENT from the map, not become offset 0")
	}
	if _, ok := index[500]; !ok {
		t.Error("the known id stays in the map")
	}
}

// TestGetSeriesIndex_CycleTerminates: a graph with a circular PREQUEL (crooked AniList data)
// can't hang the daemon pass.
func TestGetSeriesIndex_CycleTerminates(t *testing.T) {
	graph := map[int]*fixtureMedia{
		600: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 700, sequel: 700},
		700: {episodes: eps(12), format: MediaFormatTV, status: MediaStatusFinished, prequel: 600, sequel: 600},
	}
	mockSeriesAPI(t, graph)

	index, err := GetSeriesIndex([]int{600}, PriorityDisposable)
	if err != nil {
		t.Fatalf("GetSeriesIndex: %v", err)
	}
	if index[600].Offset != 12 {
		t.Errorf("the cycle has to stop after one hop: want offset 12, got %d", index[600].Offset)
	}
}
