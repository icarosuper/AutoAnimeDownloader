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
	// 150 and 300 are both in the 50+ tier: health ties and fansub decides.
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
		Codecs:        nyaa.DefaultPriorities().Codecs, // hevc first
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
	for _, source := range d.Sources {
		got := strings.ToLower(nyaa.ExtractSource("[Group] Anime - 01 [" + source + "]"))
		if got != source {
			t.Errorf("default source %q is inert: ExtractSource returns %q", source, got)
		}
	}
}

func TestExtractSource_BracketBD(t *testing.T) {
	got := nyaa.ExtractSource("[Group] Anime S01 [BD] [1080p]")
	if got != "BD" {
		t.Fatalf("ExtractSource([BD]) = %q, want BD", got)
	}
}

func TestExtractSource_BareTVInTitleIsNotASource(t *testing.T) {
	got := nyaa.ExtractSource("[SubsPlease] Lucky Star (TV) - 01 (1080p)")
	if strings.EqualFold(got, "TV") {
		t.Fatalf("ExtractSource on (TV) format marker returned %q", got)
	}
}

func TestExtractSource_TVRipAndBracketTVStillMatch(t *testing.T) {
	if got := nyaa.ExtractSource("[Group] Anime 01 [TVRip]"); !strings.EqualFold(got, "TV") {
		t.Fatalf("TVRip → TV, got %q", got)
	}
	if got := nyaa.ExtractSource("[Group] Anime 01 [TV]"); !strings.EqualFold(got, "TV") {
		t.Fatalf("[TV] → TV, got %q", got)
	}
}

func TestExtractSource_SpecificBDPatternsStillWin(t *testing.T) {
	if got := nyaa.ExtractSource("[Group] Anime [BDRip] [1080p]"); !strings.EqualFold(got, "BDRip") {
		t.Fatalf("BDRip must win over bare BD, got %q", got)
	}
	if got := nyaa.ExtractSource("[Group] Anime [BDRemux] [1080p]"); !strings.EqualFold(got, "BDRemux") {
		t.Fatalf("BDRemux must win over bare BD, got %q", got)
	}
}

func res(s string) *string { return &s }

func TestSourceCompare_UntaggedTiesWEBDL(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p)", Resolution: &r1080, Seeders: "100"},
		{Name: "[Erai-raws] Anime - 07 (1080p) WEB-DL", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortMovieResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("untagged must tie WEB-DL on source and let fansub decide, got %s", sorted[0].Name)
	}
}

func TestSourceCompare_UntaggedLosesToBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p)", Resolution: &r1080, Seeders: "100"},
		{Name: "[Group] Anime - 07 [BD] [1080p]", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortMovieResults(results)
	if !strings.Contains(sorted[0].Name, "[BD]") {
		t.Fatalf("untagged WEB must lose to [BD], got %s", sorted[0].Name)
	}
}

func TestSortTorrentResults_IgnoresSource(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime - 07 (1080p) WEB-DL", Resolution: &r1080, Seeders: "100"},
		{Name: "[Ember] Anime - 07 [BD] [1080p]", Resolution: &r1080, Seeders: "100"},
	}
	sorted := nyaa.SortTorrentResults(results)
	if !strings.Contains(sorted[0].Name, "SubsPlease") {
		t.Fatalf("RELEASING sort must ignore source and let fansub decide, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_HealthyWEBBeatsFragileBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime [BD] [1080p]", Resolution: &r1080, Seeders: "8"},
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "80"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "WEB-DL") {
		t.Fatalf("health must beat source, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_SameBandPrefersBD(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "400"},
		{Name: "[Group] Anime [BD] [1080p]", Resolution: &r1080, Seeders: "60"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "[BD]") {
		t.Fatalf("same health band must prefer BD, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_ResolutionBeforeSource(t *testing.T) {
	r1080, r720 := "1080p", "720p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime [BD] [720p]", Resolution: &r720, Seeders: "200"},
		{Name: "[SubsPlease] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "200"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(sorted[0].Name, "WEB-DL") {
		t.Fatalf("1080p WEB must beat 720p BD, got %s", sorted[0].Name)
	}
}

func TestSortFinishedSeriesResults_UncensoredFirst(t *testing.T) {
	r1080 := "1080p"
	results := []nyaa.TorrentResult{
		{Name: "[Group] Anime (1080p) WEB-DL uncensored", Resolution: &r1080, Seeders: "8"},
		{Name: "[Group] Anime (1080p) WEB-DL", Resolution: &r1080, Seeders: "80"},
	}
	sorted := nyaa.SortFinishedSeriesResults(results)
	if !strings.Contains(strings.ToLower(sorted[0].Name), "uncensored") {
		t.Fatalf("uncensored must win even on a worse health band, got %s", sorted[0].Name)
	}
}

func TestDefaultCriteriaOrder_ResolutionHealthSource(t *testing.T) {
	order := nyaa.DefaultPriorities().CriteriaOrder
	want := []string{"uncensored", "resolution", "health", "source", "codec", "fansub", "audio", "size"}
	if len(order) != len(want) {
		t.Fatalf("CriteriaOrder %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("CriteriaOrder %v, want %v", order, want)
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
