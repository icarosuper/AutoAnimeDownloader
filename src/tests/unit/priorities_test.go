package unit

import (
	"AutoAnimeDownloader/src/internal/nyaa"
	"strings"
	"testing"
)

func TestDefaultPriorities_AreNonEmpty(t *testing.T) {
	d := nyaa.DefaultPriorities()
	if len(d.Fansubs) == 0 || len(d.Resolutions) == 0 || len(d.CriteriaOrder) == 0 {
		t.Fatal("default priorities must be populated")
	}
	if d.Resolutions[0] != "1080p" || d.Resolutions[1] != "720p" {
		t.Fatalf("resolution order changed: %v", d.Resolutions[:2])
	}
	if d.Fansubs[0] != "subsplease" {
		t.Fatalf("fansub order changed: got %s", d.Fansubs[0])
	}
}

func TestSetPriorities_RestoreReturnsToDefault(t *testing.T) {
	restore := nyaa.SetPriorities(nyaa.Priorities{Fansubs: []string{"judas"}})
	if nyaa.ActivePriorities().Fansubs[0] != "judas" {
		t.Fatal("SetPriorities did not apply")
	}
	restore()
	if nyaa.ActivePriorities().Fansubs[0] != "subsplease" {
		t.Fatal("restore did not return to default")
	}
}

func TestIgnoreList_FiltersConfiguredSubstrings(t *testing.T) {
	defer nyaa.SetPriorities(nyaa.Priorities{IgnoreList: []string{"[dub]"}})()
	if !nyaa.ShouldIgnore("[Group] Anime [Dub] 1080p") {
		t.Fatal("expected [Dub] to be ignored (case-insensitive)")
	}
	if nyaa.ShouldIgnore("[Group] Anime 1080p") {
		t.Fatal("expected non-dub release to pass")
	}
	defer nyaa.SetPriorities(nyaa.Priorities{IgnoreList: nil})()
	if nyaa.ShouldIgnore("[Dub] anything") {
		t.Fatal("empty ignore list must filter nothing")
	}
}

// With the default order, health comes before fansub — but it only decides when the two
// are in different health tiers.
func TestHealthTiers_BeatFansubAcrossTiers(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 01 1080p", Resolution: &r1080, Seeders: "12"},
		{Name: "[Ember] Anime - 01 1080p", Resolution: &r1080, Seeders: "400"},
	}

	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "Ember") {
		t.Fatalf("higher health tier must beat fansub, got %s", sorted[0].Name)
	}
}

func TestHealthTiers_SameTierLetsFansubDecide(t *testing.T) {
	r1080 := "1080p"
	// 150 and 300 are both in the 100-399 tier: health ties and fansub decides.
	results := []nyaa.TorrentResult{
		{Name: "[Ember] Anime - 01 1080p", Resolution: &r1080, Seeders: "300"},
		{Name: "[SubsPlease] Anime - 01 1080p", Resolution: &r1080, Seeders: "150"},
	}

	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("within the same tier fansub must decide, got %s", sorted[0].Name)
	}
}

func TestDefaultCriteriaOrder_HealthBeforeFansub(t *testing.T) {
	order := nyaa.DefaultPriorities().CriteriaOrder
	health, fansub, resolution := -1, -1, -1
	for i, c := range order {
		switch c {
		case "health":
			health = i
		case "fansub":
			fansub = i
		case "resolution":
			resolution = i
		}
	}
	if !(resolution < health && health < fansub) {
		t.Fatalf("expected resolution < health < fansub, got %v", order)
	}
}

func TestCriteriaOrder_FansubBeforeResolution(t *testing.T) {
	// With the default order, 1080p (Ember) beats 720p (SubsPlease).
	// With fansub before resolution, SubsPlease 720p wins.
	defer nyaa.SetPriorities(nyaa.Priorities{
		CriteriaOrder: []string{"fansub", "resolution", "health", "size"},
		Fansubs:       []string{"subsplease", "ember"},
		Resolutions:   []string{"1080p", "720p"},
	})()
	r1080 := "1080p"
	r720 := "720p"
	results := []nyaa.TorrentResult{
		{Name: "[Ember] Anime 1 1080p", Resolution: &r1080},
		{Name: "[SubsPlease] Anime 1 720p", Resolution: &r720},
	}
	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("expected SubsPlease first when fansub outranks resolution, got %s", sorted[0].Name)
	}
}

// Codec enters the episode sort (episodeCriteria) and comes before fansub in the default
// order, so the codec list decides between two 1080p releases of the same health tier.
func TestCodec_DecidesEpisodeSortBeforeFansub(t *testing.T) {
	defer nyaa.SetPriorities(nyaa.Priorities{
		CriteriaOrder: nyaa.DefaultPriorities().CriteriaOrder,
		Fansubs:       nyaa.DefaultPriorities().Fansubs,
		Resolutions:   nyaa.DefaultPriorities().Resolutions,
		Codecs:        []string{"h.264", "hevc"},
	})()

	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Judas] Anime - 01 1080p x265", Resolution: &r1080, Seeders: "100"},
		{Name: "[Ember] Anime - 01 1080p x264", Resolution: &r1080, Seeders: "100"},
	}

	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "Ember") {
		t.Fatalf("x264 at the top of the codec list must beat x265, got %s", sorted[0].Name)
	}
}

// A name without a codec tag (SubsPlease's pattern) must not lose because of that:
// priorityIndex treats an unknown token as the worst, and codec runs BEFORE fansub. Without
// the codecCompare guard SubsPlease would lose to any tagged x265.
func TestCodec_UntaggedNameIsNotPenalized(t *testing.T) {
	defer nyaa.SetPriorities(nyaa.Priorities{
		CriteriaOrder: nyaa.DefaultPriorities().CriteriaOrder,
		Fansubs:       nyaa.DefaultPriorities().Fansubs,
		Resolutions:   nyaa.DefaultPriorities().Resolutions,
		Codecs:        nyaa.DefaultPriorities().Codecs, // hevc primeiro
	})()

	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Judas] Anime - 01 (1080p) [x265]", Resolution: &r1080, Seeders: "100"},
		{Name: "[SubsPlease] Anime - 01 (1080p) [ABCD1234].mkv", Resolution: &r1080, Seeders: "100"},
	}

	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("release without a codec tag must tie on codec and let fansub decide, got %s", sorted[0].Name)
	}
}

// The test that makes the "x264" bug class impossible: every token in the default lists
// must be something the extractor can return. "x265" sat in the codec list for years
// without ever matching anything, because extractCodec canonicalizes to "hevc" — inert
// config, invisible. Now it breaks CI.
func TestDefaultPriorities_TokensAreReachable(t *testing.T) {
	d := nyaa.DefaultPriorities()

	for _, codec := range d.Codecs {
		got := strings.ToLower(nyaa.ExtractCodec("[Group] Anime - 01 [" + codec + "]"))
		if got != codec {
			t.Errorf("default codec %q is inert: extractCodec returns %q", codec, got)
		}
	}
	for _, res := range d.Resolutions {
		got := strings.ToLower(nyaa.ExtractResolution("[Group] Anime - 01 (" + res + ")"))
		if got != res {
			t.Errorf("default resolution %q is inert: extractResolution returns %q", res, got)
		}
	}
}

func TestExtractResolution_Canonicalizes(t *testing.T) {
	cases := map[string]string{
		"[Group] Anime - 01 [4K]":        "2160p",
		"[Group] Anime - 01 [UHD]":       "2160p",
		"[Group] Anime - 01 [2160p]":     "2160p",
		"[Group] Anime - 01 [1920x1080]": "1080p",
		"[Group] Anime - 01 [FHD]":       "1080p",
		"[Group] Anime - 01 [1080p]":     "1080p",
		"[Group] Anime - 01 [HD]":        "720p",
	}
	for name, want := range cases {
		if got := nyaa.ExtractResolution(name); got != want {
			t.Errorf("ExtractResolution(%q) = %q, want %q", name, got, want)
		}
	}
}
