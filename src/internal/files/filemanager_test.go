package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestManagerIn(t *testing.T, dir string) *FileManager {
	t.Helper()
	return NewManager(
		NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "episodes"),
		filepath.Join(dir, "blocked"),
		filepath.Join(dir, "settings"),
		filepath.Join(dir, "standalone"),
	)
}

func newTestManager(t *testing.T) *FileManager {
	t.Helper()
	return newTestManagerIn(t, t.TempDir())
}

// O blocked_episodes antigo era um array de ids de nó da AniList. Esses ids nao existem mais em
// lugar nenhum, entao o arquivo legado tem de ser descartado — e nao virar erro, que travaria
// LoadBlockedEpisodes (e com ela o passe de verificacao) para sempre.
func TestLoadBlockedEpisodes_DiscardsLegacyIntList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blocked"), []byte("[416348,416349]"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newTestManagerIn(t, dir)

	blocked, err := m.LoadBlockedEpisodes()
	if err != nil {
		t.Fatalf("arquivo legado nao pode virar erro: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("quero lista vazia, veio %v", blocked)
	}

	// E o arquivo volta a funcionar no formato novo.
	key := EpisodeKey{AnimeID: 21, Episode: 1123}
	if err := m.BlockEpisode(key); err != nil {
		t.Fatalf("BlockEpisode: %v", err)
	}
	blocked, err = m.LoadBlockedEpisodes()
	if err != nil || len(blocked) != 1 || blocked[0] != key {
		t.Fatalf("quero [%v], veio %v (err=%v)", key, blocked, err)
	}
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
				EpisodeNumber:   1 + i,
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
			go func(ep int) {
				defer wg.Done()
				if err := m.UnmanageEpisode(EpisodeKey{AnimeID: 488911345, Episode: ep}); err != nil {
					t.Errorf("UnmanageEpisode(%d): %v", ep, err)
				}
			}(1 + i)
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
				t.Errorf("episodio %d continua manually managed: atualizacao perdida", ep.EpisodeNumber)
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
					AnimeID:       488911345,
					EpisodeNumber: 1,
					EpisodeHash:   "hash0000",
					EpisodeName:   fmt.Sprintf("reescrita %d", i),
					DownloadDate:  time.Now(),
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
			go func(ep int) {
				defer wg.Done()
				if err := m.BlockEpisode(EpisodeKey{AnimeID: 488911345, Episode: ep}); err != nil {
					t.Errorf("BlockEpisode(%d): %v", ep, err)
				}
			}(1 + i)
		}
		wg.Wait()

		blocked, err := m.LoadBlockedEpisodes()
		if err != nil {
			t.Fatalf("arquivo de bloqueados corrompido: %v", err)
		}
		if len(blocked) != n {
			t.Fatalf("quero %d episodios bloqueados, veio %d: atualizacao perdida", n, len(blocked))
		}
	})
}
