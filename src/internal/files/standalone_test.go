package files

import (
	"os"
	"testing"
)

func TestStandaloneAnimes(t *testing.T) {
	t.Run("arquivo ausente devolve lista vazia, nao erro", func(t *testing.T) {
		ids, err := newTestManager(t).LoadStandaloneAnimes()
		if err != nil {
			t.Fatalf("LoadStandaloneAnimes com arquivo ausente: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("quero lista vazia, veio %v", ids)
		}
	})

	t.Run("Add e idempotente", func(t *testing.T) {
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
			t.Fatalf("quero [21], veio %v", ids)
		}
	})

	t.Run("Remove de id ausente nao e erro", func(t *testing.T) {
		m := newTestManager(t)
		if err := m.AddStandaloneAnime(21); err != nil {
			t.Fatalf("AddStandaloneAnime: %v", err)
		}
		if err := m.RemoveStandaloneAnime(999); err != nil {
			t.Fatalf("RemoveStandaloneAnime de id ausente: %v", err)
		}

		ids, _ := m.LoadStandaloneAnimes()
		if len(ids) != 1 {
			t.Fatalf("remover id ausente mexeu na lista: %v", ids)
		}

		if err := m.RemoveStandaloneAnime(21); err != nil {
			t.Fatalf("RemoveStandaloneAnime: %v", err)
		}
		ids, _ = m.LoadStandaloneAnimes()
		if len(ids) != 0 {
			t.Fatalf("quero lista vazia depois do remove, veio %v", ids)
		}
	})

	t.Run("grava array JSON, mesma convencao de blocked_episodes", func(t *testing.T) {
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
			t.Fatalf("quero [21,154587], veio %q", string(b))
		}
	})
}
