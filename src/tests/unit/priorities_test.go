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

func TestCriteriaOrder_FansubBeforeResolution(t *testing.T) {
	// Com ordem default, 1080p (Ember) vence 720p (SubsPlease).
	// Com fansub antes de resolution, SubsPlease 720p vence.
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
