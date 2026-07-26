package files

import (
	"os"
	"path/filepath"
	"testing"
)

func intPtr(i int) *int { return &i }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestJellyfinName(t *testing.T) {
	cases := []struct {
		anime string
		ep    int
		ext   string
		want  string
	}{
		{"My Anime", 5, ".mkv", "My Anime - E05.mkv"},
		{"My Anime", 12, ".mp4", "My Anime - E12.mp4"},
		{"Anime: Colon", 1, ".mkv", "Anime Colon - E01.mkv"},
	}
	for _, c := range cases {
		if got := jellyfinName(c.anime, c.ep, c.ext); got != c.want {
			t.Errorf("jellyfinName(%q,%d,%q) = %q, want %q", c.anime, c.ep, c.ext, got, c.want)
		}
	}
}

func TestSanitizeFolderName(t *testing.T) {
	cases := map[string]string{
		"My Anime Season 2": "My Anime",
		"Anime/Name:Test":   "AnimeNameTest",
		"Show 2nd Season":   "Show",
	}
	for in, want := range cases {
		if got := sanitizeFolderName(in); got != want {
			t.Errorf("sanitizeFolderName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrganizeSingleEpisodeJellyfin(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "torrentid")
	completed := filepath.Join(tmp, "completed")
	src := filepath.Join(dataDir, "raw episode name.mkv")
	writeFile(t, src, "video-bytes")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "My Anime",
		CompletedPath:  completed,
		EpisodeNumber:  intPtr(5),
		RenameJellyfin: true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}

	wantDest := filepath.Join(completed, "My Anime", "My Anime - E05.mkv")
	if len(created) != 1 || created[0] != wantDest {
		t.Fatalf("created = %v, want [%s]", created, wantDest)
	}

	// Proof of hardlink: same inode as source (not a copy), source still present.
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatalf("source missing after organize: %v", err)
	}
	destInfo, err := os.Stat(wantDest)
	if err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if !os.SameFile(srcInfo, destInfo) {
		t.Errorf("dest is not a hardlink of src (different inode)")
	}
}

func TestOrganizeBatchRawNames(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "Anime - 01.mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "Anime - 02.mkv"), "b")
	writeFile(t, filepath.Join(dataDir, "readme.txt"), "not video")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		IsBatch:        true,
		RenameJellyfin: true, // ignored for batch
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %v, want 2 video links (txt skipped)", created)
	}
	for _, name := range []string{"Anime - 01.mkv", "Anime - 02.mkv"} {
		p := filepath.Join(completed, "Anime", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected raw-named link %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(completed, "Anime", "readme.txt")); err == nil {
		t.Errorf("non-video file should not be linked")
	}
}

func TestOrganizeIdempotent(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "id")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "ep.mkv"), "x")

	lib := NewLibrarian(NewOSFileSystem())
	req := OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	}
	first, err := lib.Organize(req)
	if err != nil {
		t.Fatalf("first Organize: %v", err)
	}
	second, err := lib.Organize(req)
	if err != nil {
		t.Fatalf("second Organize (idempotent): %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0] != second[0] {
		t.Errorf("idempotent mismatch: first=%v second=%v", first, second)
	}
}

func TestOrganizeNoVideoFiles(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "id")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "readme.txt"), "x")

	lib := NewLibrarian(NewOSFileSystem())
	_, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	})
	if err == nil {
		t.Fatalf("expected error when no video files present")
	}
}

func TestRemoveFromLibrary(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "lib", "a.mkv")
	writeFile(t, target, "x")

	lib := NewLibrarian(NewOSFileSystem())
	if err := lib.RemoveFromLibrary(target); err != nil {
		t.Fatalf("RemoveFromLibrary: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("file should be removed")
	}
	// Removing again (missing) is not an error.
	if err := lib.RemoveFromLibrary(target); err != nil {
		t.Errorf("RemoveFromLibrary on missing path should be nil, got %v", err)
	}
	// Empty path is a no-op.
	if err := lib.RemoveFromLibrary(""); err != nil {
		t.Errorf("RemoveFromLibrary(\"\") should be nil, got %v", err)
	}
}

func TestProbePathsSameVolume(t *testing.T) {
	tmp := t.TempDir()
	save := filepath.Join(tmp, "save")
	completed := filepath.Join(tmp, "completed")

	lib := NewLibrarian(NewOSFileSystem())
	if err := lib.ProbePaths(save, completed); err != nil {
		t.Fatalf("ProbePaths same volume: %v", err)
	}
	// No probe leftovers.
	if _, err := os.Stat(filepath.Join(save, ".aad_link_probe")); err == nil {
		t.Errorf("probe source not cleaned up")
	}
	if _, err := os.Stat(filepath.Join(completed, ".aad_link_probe")); err == nil {
		t.Errorf("probe dest not cleaned up")
	}
}

func TestProbePathsEmpty(t *testing.T) {
	lib := NewLibrarian(NewOSFileSystem())
	if err := lib.ProbePaths("", "/x"); err == nil {
		t.Errorf("expected error for empty save path")
	}
}
