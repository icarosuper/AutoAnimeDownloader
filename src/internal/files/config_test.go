package files

import (
	"path/filepath"
	"testing"
)

func TestConfigDownloadPath(t *testing.T) {
	t.Run("derives the download directory from the library", func(t *testing.T) {
		c := &Config{CompletedAnimePath: filepath.Join("/mnt", "hd", "animes")}
		want := filepath.Join("/mnt", "hd", "animes", ".torrents")
		if got := c.DownloadPath(); got != want {
			t.Errorf("DownloadPath() = %q, want %q", got, want)
		}
	})

	// Critical guard: without it filepath.Join would return the relative path
	// ".torrents" and the rain session would be created in the process
	// working directory. With "", SessionManager.Ensure returns
	// ErrSessionNotReady, which is the current behavior for incomplete config.
	t.Run("empty library returns empty string", func(t *testing.T) {
		c := &Config{CompletedAnimePath: ""}
		if got := c.DownloadPath(); got != "" {
			t.Errorf("DownloadPath() = %q, want empty string", got)
		}
	})
}
