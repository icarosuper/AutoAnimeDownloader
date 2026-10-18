package files

import (
	"os"
	"testing"
)

func TestStandaloneAnimes(t *testing.T) {
	t.Run("missing file returns empty list, not error", func(t *testing.T) {
		ids, err := newTestManager(t).LoadStandaloneAnimes()
		if err != nil {
			t.Fatalf("LoadStandaloneAnimes with missing file: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("want empty list, got %v", ids)
		}
	})

	t.Run("Add is idempotent", func(t *testing.T) {
		m := newTestManager(t)
		for range 3 {
			if err := m.AddStandaloneAnime(21); err != nil {
				t.Fatalf("AddStandaloneAnime: %v", err)
			}
		}

		ids, err := m.LoadStandaloneAnimes()
		if err != nil {
			t.Fatalf("LoadStandaloneAnimes: %v", err)
		}
		if len(ids) != 1 || ids[0] != 21 {
			t.Fatalf("want [21], got %v", ids)
		}
	})

	t.Run("Remove of missing id is not an error", func(t *testing.T) {
		m := newTestManager(t)
		if err := m.AddStandaloneAnime(21); err != nil {
			t.Fatalf("AddStandaloneAnime: %v", err)
		}
		if err := m.RemoveStandaloneAnime(999); err != nil {
			t.Fatalf("RemoveStandaloneAnime of missing id: %v", err)
		}

		ids, _ := m.LoadStandaloneAnimes()
		if len(ids) != 1 {
			t.Fatalf("removing missing id changed the list: %v", ids)
		}

		if err := m.RemoveStandaloneAnime(21); err != nil {
			t.Fatalf("RemoveStandaloneAnime: %v", err)
		}
		ids, _ = m.LoadStandaloneAnimes()
		if len(ids) != 0 {
			t.Fatalf("want empty list after remove, got %v", ids)
		}
	})

	t.Run("writes a JSON array, same convention as blocked_episodes", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/standalone"
		m := NewManager(
			NewOSFileSystem(),
			dir+"/config.json",
			dir+"/episodes",
			dir+"/blocked",
			dir+"/settings",
			path,
		)
		if err := m.AddStandaloneAnime(21); err != nil {
			t.Fatalf("AddStandaloneAnime: %v", err)
		}
		if err := m.AddStandaloneAnime(154587); err != nil {
			t.Fatalf("AddStandaloneAnime: %v", err)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(b) != "[21,154587]" {
			t.Fatalf("want [21,154587], got %q", string(b))
		}
	})
}
