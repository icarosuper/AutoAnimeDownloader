package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *FileManager {
	t.Helper()
	dir := t.TempDir()
	return NewManager(
		NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "episodes"),
		filepath.Join(dir, "blocked"),
		filepath.Join(dir, "settings"),
	)
}

func TestDeleteEmptyFolders(t *testing.T) {
	t.Run("apaga pasta vazia da biblioteca", func(t *testing.T) {
		completed := t.TempDir()
		vazia := filepath.Join(completed, "Anime Vazio")
		if err := os.MkdirAll(vazia, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		if _, err := os.Stat(vazia); err == nil {
			t.Error("pasta vazia deveria ter sido apagada")
		}
	})

	// A rain aloca <download>/<id> e so depois escreve nele. Sem esta guarda a varredura
	// da biblioteca apagaria diretorios de torrent recem-criados.
	t.Run("nunca entra no diretorio de download", func(t *testing.T) {
		completed := t.TempDir()
		torrentDir := filepath.Join(completed, ".torrents", "abc123")
		if err := os.MkdirAll(torrentDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := newTestManager(t).DeleteEmptyFolders(completed); err != nil {
			t.Fatalf("DeleteEmptyFolders: %v", err)
		}

		if _, err := os.Stat(torrentDir); err != nil {
			t.Errorf("diretorio de torrent vazio nao deveria ter sido apagado: %v", err)
		}
	})

	t.Run("rejeita caminho vazio", func(t *testing.T) {
		if err := newTestManager(t).DeleteEmptyFolders(""); err == nil {
			t.Error("quero erro para caminho vazio, veio nil")
		}
	})
}

// Regressao: soltar varios episodios de uma vez pela UI dispara um POST /release por
// episodio em paralelo, e cada um faz load -> altera -> grava o arquivo de episodios.
// Sem lock isso perde atualizacoes e chega a corromper o arquivo (dois WriteFile
// truncando o mesmo caminho ao mesmo tempo), deixando LoadSavedEpisodes falhando para
// sempre e derrubando /api/v1/animes com 500.
func TestEpisodesFileConcurrency(t *testing.T) {
	seed := func(t *testing.T, m *FileManager, n int) {
		t.Helper()
		episodes := make([]EpisodeStruct, 0, n)
		for i := 0; i < n; i++ {
			episodes = append(episodes, EpisodeStruct{
				EpisodeID:       1000 + i,
				EpisodeHash:     fmt.Sprintf("hash%04d", i),
				EpisodeName:     fmt.Sprintf("Anime Bem Longo Para Encher o Arquivo - Episode %d", i),
				AnimeID:         488911345,
				AnimeName:       "Katainaka no Ossan, Kensei ni Naru II",
				DownloadDate:    time.Now(),
				ManuallyManaged: true,
			})
		}
		if err := m.SaveEpisodesToFile(episodes); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	t.Run("releases concorrentes nao perdem atualizacoes nem corrompem o arquivo", func(t *testing.T) {
		const n = 30
		m := newTestManager(t)
		seed(t, m, n)

		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				if err := m.UnmanageEpisode(id); err != nil {
					t.Errorf("UnmanageEpisode(%d): %v", id, err)
				}
			}(1000 + i)
		}
		wg.Wait()

		got, err := m.LoadSavedEpisodes()
		if err != nil {
			t.Fatalf("arquivo de episodios corrompido: %v", err)
		}
		if len(got) != n {
			t.Fatalf("quero %d episodios salvos, veio %d", n, len(got))
		}
		for _, ep := range got {
			if ep.ManuallyManaged {
				t.Errorf("episodio %d continua manually managed: atualizacao perdida", ep.EpisodeID)
			}
		}
	})

	// O daemon organiza torrents (UpsertEpisodes) enquanto a UI faz polling em
	// /api/v1/animes (LoadSavedEpisodes). Um leitor nunca pode enxergar o arquivo
	// pela metade.
	t.Run("leitura concorrente com escrita nunca ve arquivo truncado", func(t *testing.T) {
		const n = 40
		m := newTestManager(t)
		seed(t, m, n)

		done := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done)
			for i := 0; i < 50; i++ {
				if err := m.UpsertEpisodes([]EpisodeStruct{{
					EpisodeID:    1000,
					EpisodeHash:  "hash0000",
					EpisodeName:  fmt.Sprintf("reescrita %d", i),
					DownloadDate: time.Now(),
				}}); err != nil {
					t.Errorf("UpsertEpisodes: %v", err)
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				eps, err := m.LoadSavedEpisodes()
				if err != nil {
					t.Errorf("leitura pegou arquivo corrompido: %v", err)
					return
				}
				if len(eps) != n {
					t.Errorf("leitura pegou arquivo incompleto: quero %d episodios, veio %d", n, len(eps))
					return
				}
			}
		}()

		wg.Wait()
	})

	t.Run("bloqueio e desbloqueio concorrentes nao corrompem o arquivo", func(t *testing.T) {
		const n = 30
		m := newTestManager(t)

		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				if err := m.BlockEpisode(id); err != nil {
					t.Errorf("BlockEpisode(%d): %v", id, err)
				}
			}(2000 + i)
		}
		wg.Wait()

		ids, err := m.LoadBlockedEpisodes()
		if err != nil {
			t.Fatalf("arquivo de bloqueados corrompido: %v", err)
		}
		if len(ids) != n {
			t.Fatalf("quero %d episodios bloqueados, veio %d: atualizacao perdida", n, len(ids))
		}
	})
}
