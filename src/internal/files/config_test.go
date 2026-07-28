package files

import (
	"path/filepath"
	"testing"
)

func TestConfigDownloadPath(t *testing.T) {
	t.Run("deriva o diretorio de download da biblioteca", func(t *testing.T) {
		c := &Config{CompletedAnimePath: filepath.Join("/mnt", "hd", "animes")}
		want := filepath.Join("/mnt", "hd", "animes", ".torrents")
		if got := c.DownloadPath(); got != want {
			t.Errorf("DownloadPath() = %q, quero %q", got, want)
		}
	})

	// Guarda critica: sem ela filepath.Join devolveria o caminho relativo
	// ".torrents" e a sessao da rain seria criada no diretorio de
	// trabalho do processo. Com "", SessionManager.Ensure devolve
	// ErrSessionNotReady, que e o comportamento atual para config incompleta.
	t.Run("biblioteca vazia devolve string vazia", func(t *testing.T) {
		c := &Config{CompletedAnimePath: ""}
		if got := c.DownloadPath(); got != "" {
			t.Errorf("DownloadPath() = %q, quero string vazia", got)
		}
	})
}
