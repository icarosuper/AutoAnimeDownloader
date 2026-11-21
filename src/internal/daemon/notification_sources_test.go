package daemon

import (
	"AutoAnimeDownloader/src/internal/files"
	"slices"
	"testing"
)

// An anime's sources are the accounts whose last fetched lists hold it — download or delete kind —
// plus "standalone" from the standalone file. An id no list holds and that isn't standalone stays
// out of the map: unknown, so the filtered webhooks get it (fail-open). An account dropped from the
// config no longer counts, and a failed fetch (nothing recorded) keeps the previous answer.
func TestNotificationSources(t *testing.T) {
	saved := passLists
	passLists = &accountLists{byID: make(map[listKind]map[string]map[int]bool)}
	t.Cleanup(func() { passLists = saved })

	passLists.record(listDownload, "anilist:alice", map[int]bool{21: true})
	passLists.record(listDelete, "anilist:bob", map[int]bool{21: true, 55: true})
	passLists.record(listDownload, "mal:carol", map[int]bool{55: true})
	passLists.record(listDownload, "anilist:gone", map[int]bool{44: true})

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{21, 33}}
	cfg := &files.Config{AnilistUsernames: []string{"alice", "bob"}, MALUsernames: []string{"carol"}}

	got := NotificationSources(fm)(cfg, []int{21, 33, 44, 55})

	if want := []string{"anilist:alice", "anilist:bob", "standalone"}; !slices.Equal(got[21], want) {
		t.Errorf("anime 21: got %v, want %v", got[21], want)
	}
	if want := []string{"standalone"}; !slices.Equal(got[33], want) {
		t.Errorf("anime 33: got %v, want %v", got[33], want)
	}
	if _, ok := got[44]; ok {
		t.Errorf("an anime only a removed account held must be unknown, got %v", got[44])
	}
	if want := []string{"anilist:bob", "mal:carol"}; !slices.Equal(got[55], want) {
		t.Errorf("anime 55: got %v, want %v", got[55], want)
	}

	// A new answer replaces that account's list of that kind only.
	passLists.record(listDownload, "anilist:alice", map[int]bool{})
	if got := NotificationSources(fm)(cfg, []int{21}); !slices.Equal(got[21], []string{"anilist:bob", "standalone"}) {
		t.Errorf("after alice dropped it: got %v", got[21])
	}
}

// The delete half only holds what is on disk, so an anime the pass deletes would lose its owner
// on the next pass while its episode_deleted may still sit in a batch window. recordCarrying keeps
// it one more pass, and only one.
func TestRecordCarryingKeepsADeletedAnimeForOnePass(t *testing.T) {
	l := &accountLists{byID: make(map[listKind]map[string]map[int]bool)}
	acc := "anilist:alice"

	l.recordCarrying(listDelete, acc, map[int]bool{7: true})
	l.recordCarrying(listDelete, acc, map[int]bool{}) // 7 deleted, no longer on disk
	if got := l.owners([]string{acc}, []int{7}); len(got[7]) != 1 {
		t.Fatalf("the pass right after the deletion must still know 7's owner, got %v", got)
	}
	l.recordCarrying(listDelete, acc, map[int]bool{})
	if got := l.owners([]string{acc}, []int{7}); len(got[7]) != 0 {
		t.Errorf("two passes later 7 must be gone, got %v", got)
	}
}
