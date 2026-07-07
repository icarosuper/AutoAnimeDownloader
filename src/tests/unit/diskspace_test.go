package unit

import (
	"os"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
)

func TestDiskSpace(t *testing.T) {
	t.Run("valid path returns total >= free > 0", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "aad_test_disk_")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmp)

		total, free, err := files.DiskSpace(tmp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total == 0 {
			t.Error("expected total > 0")
		}
		if free > total {
			t.Errorf("expected free (%d) <= total (%d)", free, total)
		}
	})

	t.Run("nonexistent path returns error", func(t *testing.T) {
		_, _, err := files.DiskSpace("/definitely/does/not/exist/aad")
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})
}
