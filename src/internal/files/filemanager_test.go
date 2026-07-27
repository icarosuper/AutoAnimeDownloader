package files

import (
	"os"
	"path/filepath"
	"testing"
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
		torrentDir := filepath.Join(completed, ".autoAnimeDownloader", "abc123")
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
