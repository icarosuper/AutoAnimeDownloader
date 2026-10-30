package files

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func intPtr(i int) *int { return &i }

// linkCount reports the hardlink count of path. The syscall struct behind FileInfo.Sys()
// is platform-specific (and has no Nlink at all on Windows), so it is read reflectively:
// ok == false means "this platform does not expose it" and the caller should skip.
func linkCount(t *testing.T, path string) (int, bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	sys := info.Sys()
	if sys == nil {
		return 0, false
	}
	v := reflect.Indirect(reflect.ValueOf(sys))
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	field := v.FieldByName("Nlink")
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(field.Uint()), true
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int()), true
	}
	return 0, false
}

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

// The season marker MUST survive: each season is its own AniList entry, with its own
// id and cover. Stripping the marker dumped Season 1/2/3 into the same folder and Jellyfin
// matched everything to season 1 (decisions.md #45).
func TestSanitizeNameKeepsSeasonMarker(t *testing.T) {
	cases := map[string]string{
		"Anime/Name:Test":                                       "AnimeNameTest",
		"My Anime Season 2":                                     "My Anime Season 2",
		"Show 2nd Season":                                       "Show 2nd Season",
		"Mushoku Tensei: Jobless Reincarnation":                 "Mushoku Tensei Jobless Reincarnation",
		"Mushoku Tensei: Jobless Reincarnation Season 3":        "Mushoku Tensei Jobless Reincarnation Season 3",
		"Mushoku Tensei: Jobless Reincarnation Cour 2":          "Mushoku Tensei Jobless Reincarnation Cour 2",
		"Mushoku Tensei: Jobless Reincarnation Season 2 Part 2": "Mushoku Tensei Jobless Reincarnation Season 2 Part 2",
	}
	seen := make(map[string]string)
	for in, want := range cases {
		got := sanitizeName(in)
		if got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%q and %q land in the same folder %q", other, in, got)
		}
		seen[got] = in
	}
}

func TestOrganizeWritesShowNFO(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "torrentid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "ep.mkv"), "video-bytes")

	lib := NewLibrarian(NewOSFileSystem())
	req := OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "My Anime",
		AnimeID:        194829,
		CompletedPath:  completed,
	}
	if _, err := lib.Organize(req); err != nil {
		t.Fatalf("Organize: %v", err)
	}

	nfo := filepath.Join(completed, "My Anime", "tvshow.nfo")
	data, err := os.ReadFile(nfo)
	if err != nil {
		t.Fatalf("tvshow.nfo missing: %v", err)
	}
	if !strings.Contains(string(data), `<uniqueid type="AniList" default="true">194829</uniqueid>`) {
		t.Errorf("nfo without anilist uniqueid:\n%s", data)
	}

	// Does not overwrite an existing nfo (match adjusted by hand by the user).
	if err := os.WriteFile(nfo, []byte("manual"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Organize(req); err != nil {
		t.Fatalf("Organize (2nd time): %v", err)
	}
	if data, _ := os.ReadFile(nfo); string(data) != "manual" {
		t.Errorf("existing nfo was overwritten: %s", data)
	}

	// Without AnimeID it writes nothing.
	req.AnimeName = "Another Anime"
	req.AnimeID = 0
	if _, err := lib.Organize(req); err != nil {
		t.Fatalf("Organize (no id): %v", err)
	}
	if _, err := os.Stat(filepath.Join(completed, "Another Anime", "tvshow.nfo")); err == nil {
		t.Error("wrote tvshow.nfo without AnimeID")
	}
}

func TestBackfillShowNFOs(t *testing.T) {
	tmp := t.TempDir()
	existing := filepath.Join(tmp, "completed", "My Anime")
	writeFile(t, filepath.Join(existing, "My Anime - E01.mkv"), "video-bytes")
	gone := filepath.Join(tmp, "completed", "Deleted Anime")

	lib := NewLibrarian(NewOSFileSystem())
	lib.BackfillShowNFOs([]EpisodeStruct{
		{AnimeID: 194829, AnimeName: "My Anime", LibraryPaths: []string{filepath.Join(existing, "My Anime - E01.mkv")}},
		// Second episode of the same anime: must not duplicate work or fail.
		{AnimeID: 194829, AnimeName: "My Anime", LibraryPaths: []string{filepath.Join(existing, "My Anime - E02.mkv")}},
		// Folder that vanished from disk.
		{AnimeID: 111, AnimeName: "Deleted Anime", LibraryPaths: []string{filepath.Join(gone, "ep.mkv")}},
		// No id and no paths: ignored.
		{AnimeID: 0, AnimeName: "No ID", LibraryPaths: []string{filepath.Join(existing, "x.mkv")}},
		{AnimeID: 222, AnimeName: "Not Organized"},
	})

	data, err := os.ReadFile(filepath.Join(existing, "tvshow.nfo"))
	if err != nil {
		t.Fatalf("tvshow.nfo was not created by the backfill: %v", err)
	}
	if !strings.Contains(string(data), ">194829<") {
		t.Errorf("nfo with the wrong id:\n%s", data)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Error("backfill created the folder of an anime that is no longer on disk")
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
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 01 [1080p].mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 02 [1080p].mkv"), "b")
	writeFile(t, filepath.Join(dataDir, "readme.txt"), "not video")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		IsBatch:        true,
		RenameJellyfin: false,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %v, want 2 video links (txt skipped)", created)
	}
	for _, name := range []string{"[Sub] Anime - 01 [1080p].mkv", "[Sub] Anime - 02 [1080p].mkv"} {
		p := filepath.Join(completed, "Anime", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected raw-named link %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(completed, "Anime", "readme.txt")); err == nil {
		t.Errorf("non-video file should not be linked")
	}
}

// With the flag on, pack files get the SAME name as the singles, in the same
// anime folder. Anything without a readable number (NCOP) and the second file that lands on
// the same number keep the raw name — renaming both would collide and one would overwrite the other.
func TestOrganizeBatchJellyfinNames(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 01 [1080p].mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 02 [1080p].mkv"), "b")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 02v2 [1080p].mkv"), "c")
	writeFile(t, filepath.Join(dataDir, "NCOP.mkv"), "d")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		IsBatch:        true,
		RenameJellyfin: true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 4 {
		t.Fatalf("created = %v, want 4 links", created)
	}
	for _, name := range []string{
		"Anime - E01.mkv",
		"Anime - E02.mkv",
		"[Sub] Anime - 02v2 [1080p].mkv", // collision with E02: raw name
		"NCOP.mkv",                       // no number: raw name
	} {
		if _, err := os.Stat(filepath.Join(completed, "Anime", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

// Group that separates the name with "_": the episode number comes out of the file the same
// as from a name with spaces, otherwise the whole pack would go into the library with the raw name.
func TestOrganizeBatchUnderscoreNames(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "[DB]Anime_-_01_(Dual Audio_10bit_BD1080p_x265).mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[DB]Anime_-_02_(Dual Audio_10bit_BD1080p_x265).mkv"), "b")
	writeFile(t, filepath.Join(dataDir, "[DB]Anime_-_NCED01_(10bit_BD1080p_x265).mkv"), "c")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		IsBatch:        true,
		RenameJellyfin: true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("created = %v, want 3 links", created)
	}
	for _, name := range []string{
		"Anime - E01.mkv",
		"Anime - E02.mkv",
		"[DB]Anime_-_NCED01_(10bit_BD1080p_x265).mkv", // no episode number: raw name
	} {
		if _, err := os.Stat(filepath.Join(completed, "Anime", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

// Pack and single of the same anime share ONE folder (destDir comes from AnimeName, never from
// the torrent name) and, with the flag, the same name pattern — so the episodes mix in order.
func TestOrganizeBatchAndSingleShareOneFolder(t *testing.T) {
	tmp := t.TempDir()
	completed := filepath.Join(tmp, "completed")

	batchDir := filepath.Join(tmp, "save", "batch")
	writeFile(t, filepath.Join(batchDir, "[Sub] Anime - 01 [1080p].mkv"), "a")
	singleDir := filepath.Join(tmp, "save", "single")
	writeFile(t, filepath.Join(singleDir, "[Other] Anime - 02 [720p].mkv"), "b")

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: batchDir, AnimeName: "Anime", CompletedPath: completed,
		IsBatch: true, RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize batch: %v", err)
	}
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: singleDir, AnimeName: "Anime", CompletedPath: completed,
		EpisodeNumber: intPtr(2), RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize single: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(completed, "Anime"))
	if err != nil {
		t.Fatalf("read library folder: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{"Anime - E01.mkv", "Anime - E02.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("library folder = %v, want %v", got, want)
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

// Destination exists and is the very same file (already hardlinked): true no-op, the
// path is still reported, and the link count does not grow.
func TestOrganizeSameFileIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "id")
	completed := filepath.Join(tmp, "completed")
	src := filepath.Join(dataDir, "ep.mkv")
	writeFile(t, src, "video-bytes")

	lib := NewLibrarian(NewOSFileSystem())
	req := OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	}
	first, err := lib.Organize(req)
	if err != nil {
		t.Fatalf("first Organize: %v", err)
	}
	dest := filepath.Join(completed, "A", "A - E01.mkv")
	if len(first) != 1 || first[0] != dest {
		t.Fatalf("created = %v, want [%s]", first, dest)
	}
	before, ok := linkCount(t, src)
	if ok && before != 2 {
		t.Fatalf("link count after first Organize = %d, want 2", before)
	}

	second, err := lib.Organize(req)
	if err != nil {
		t.Fatalf("second Organize: %v", err)
	}
	if len(second) != 1 || second[0] != dest {
		t.Fatalf("second created = %v, want [%s]", second, dest)
	}
	if after, ok2 := linkCount(t, src); ok && ok2 && after != before {
		t.Errorf("link count changed on no-op: %d -> %d", before, after)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatalf("source missing: %v", err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if !os.SameFile(srcInfo, destInfo) {
		t.Errorf("dest should still be the same file as src")
	}
}

// Destination exists but points at different bytes (redownload/replace): the new file
// wins, and the seeded source keeps exactly one extra link.
func TestOrganizeReplacesDifferentFileAtDestination(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "newid")
	completed := filepath.Join(tmp, "completed")
	src := filepath.Join(dataDir, "new release.mkv")
	writeFile(t, src, "new-bytes")

	// A stale, unrelated file already sitting at the destination name.
	dest := filepath.Join(completed, "A", "A - E01.mkv")
	writeFile(t, dest, "stale-bytes")
	// Keep a second name for the stale file so its identity outlives the replacement.
	// On Windows os.Stat records only the path and os.SameFile resolves the file id
	// lazily by reopening it, so a FileInfo taken from dest before the swap would
	// compare equal to whatever sits at dest afterwards. Statting the alias instead
	// pins the original file on every platform.
	staleAlias := filepath.Join(tmp, "stale-alias.mkv")
	if err := os.Link(dest, staleAlias); err != nil {
		t.Fatalf("link stale alias: %v", err)
	}

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 1 || created[0] != dest {
		t.Fatalf("created = %v, want [%s]", created, dest)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatalf("source missing after replacement: %v", err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("dest missing after replacement: %v", err)
	}
	if !os.SameFile(srcInfo, destInfo) {
		t.Errorf("dest was not relinked to src")
	}
	staleInfo, err := os.Stat(staleAlias)
	if err != nil {
		t.Fatalf("stale alias missing after replacement: %v", err)
	}
	if os.SameFile(staleInfo, destInfo) {
		t.Errorf("dest still points at the stale file")
	}
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(content) != "new-bytes" {
		t.Errorf("dest content = %q, want %q", content, "new-bytes")
	}
	// The seeded file must survive with exactly one extra name (seeding not broken).
	if n, ok := linkCount(t, src); ok && n != 2 {
		t.Errorf("source link count = %d, want 2", n)
	}
}

// An empty CompletedPath must fail loudly instead of hardlinking into the process CWD.
func TestOrganizeEmptyCompletedPath(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "save", "id")
	writeFile(t, filepath.Join(dataDir, "ep.mkv"), "x")

	cwd := t.TempDir()
	t.Chdir(cwd)

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "My Anime", CompletedPath: "",
		EpisodeNumber: intPtr(1), RenameJellyfin: true,
	})
	if err == nil {
		t.Fatalf("expected error for empty completed path, created = %v", created)
	}
	entries, readErr := os.ReadDir(cwd)
	if readErr != nil {
		t.Fatalf("read cwd: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("Organize wrote %d entries into the working directory: %v", len(entries), entries)
	}
}

// EpisodeNumber == 0 is missing data, never a real episode: fall back to the raw name.
func TestOrganizeEpisodeNumberZeroKeepsRawName(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "id")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "raw episode name.mkv"), "x")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir, AnimeName: "A", CompletedPath: completed,
		EpisodeNumber: intPtr(0), RenameJellyfin: true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	want := filepath.Join(completed, "A", "raw episode name.mkv")
	if len(created) != 1 || created[0] != want {
		t.Fatalf("created = %v, want [%s]", created, want)
	}
	if _, err := os.Stat(filepath.Join(completed, "A", "A - E00.mkv")); err == nil {
		t.Errorf("episode 0 must never produce an E00 name")
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
	lib := NewLibrarian(NewOSFileSystem())
	assertMissing := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be gone (%s): %v", what, path, err)
		}
	}
	assertPresent := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should still exist (%s): %v", what, path, err)
		}
	}

	t.Run("video and sibling nfo both go", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "lib")
		video := filepath.Join(dir, "Anime - E05.mkv")
		nfo := filepath.Join(dir, "Anime - E05.nfo")
		writeFile(t, video, "video")
		writeFile(t, nfo, "nfo")

		if err := lib.RemoveFromLibrary(video); err != nil {
			t.Fatalf("RemoveFromLibrary: %v", err)
		}
		assertMissing(t, video, "video")
		assertMissing(t, nfo, "sidecar nfo")
	})

	t.Run("video only is not an error", func(t *testing.T) {
		video := filepath.Join(t.TempDir(), "lib", "a.mkv")
		writeFile(t, video, "x")

		if err := lib.RemoveFromLibrary(video); err != nil {
			t.Fatalf("RemoveFromLibrary: %v", err)
		}
		assertMissing(t, video, "video")
	})

	t.Run("nfo only when video already missing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "lib")
		video := filepath.Join(dir, "Anime - E05.mkv")
		nfo := filepath.Join(dir, "Anime - E05.nfo")
		writeFile(t, nfo, "nfo")

		if err := lib.RemoveFromLibrary(video); err != nil {
			t.Fatalf("RemoveFromLibrary on missing video: %v", err)
		}
		assertMissing(t, nfo, "sidecar nfo")
	})

	t.Run("path that is itself an nfo does not hunt a further sidecar", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "lib")
		nfo := filepath.Join(dir, "foo.nfo")
		// A sibling "foo" with no extension must not be treated as a sidecar target.
		other := filepath.Join(dir, "foo")
		writeFile(t, nfo, "nfo")
		writeFile(t, other, "keep")

		if err := lib.RemoveFromLibrary(nfo); err != nil {
			t.Fatalf("RemoveFromLibrary: %v", err)
		}
		assertMissing(t, nfo, "nfo")
		assertPresent(t, other, "non-nfo sibling")
	})

	t.Run("empty path is a no-op", func(t *testing.T) {
		if err := lib.RemoveFromLibrary(""); err != nil {
			t.Errorf("RemoveFromLibrary(\"\") should be nil, got %v", err)
		}
	})
}

func TestRemoveFolderIfNoVideo(t *testing.T) {
	lib := NewLibrarian(NewOSFileSystem())
	assertMissing := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be gone (%s): %v", what, path, err)
		}
	}
	assertPresent := func(t *testing.T, path, what string) {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should still exist (%s): %v", what, path, err)
		}
	}

	t.Run("nfo-only folder is wiped", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Anime")
		writeFile(t, filepath.Join(dir, "tvshow.nfo"), "show")
		writeFile(t, filepath.Join(dir, "Anime - E01.nfo"), "ep")

		if err := lib.RemoveFolderIfNoVideo(dir); err != nil {
			t.Fatalf("RemoveFolderIfNoVideo: %v", err)
		}
		assertMissing(t, dir, "library folder")
	})

	t.Run("folder with a video is left untouched", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Anime")
		video := filepath.Join(dir, "Anime - E01.mkv")
		nfo := filepath.Join(dir, "tvshow.nfo")
		writeFile(t, video, "video")
		writeFile(t, nfo, "show")

		if err := lib.RemoveFolderIfNoVideo(dir); err != nil {
			t.Fatalf("RemoveFolderIfNoVideo: %v", err)
		}
		assertPresent(t, dir, "library folder")
		assertPresent(t, video, "video")
		assertPresent(t, nfo, "tvshow.nfo")
	})

	t.Run("poster-only folder is wiped", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Anime")
		writeFile(t, filepath.Join(dir, "poster.jpg"), "art")

		if err := lib.RemoveFolderIfNoVideo(dir); err != nil {
			t.Fatalf("RemoveFolderIfNoVideo: %v", err)
		}
		assertMissing(t, dir, "library folder")
	})

	t.Run("missing directory is not an error", func(t *testing.T) {
		if err := lib.RemoveFolderIfNoVideo(filepath.Join(t.TempDir(), "gone")); err != nil {
			t.Errorf("missing directory should be nil, got %v", err)
		}
	})

	t.Run("empty path is a no-op", func(t *testing.T) {
		if err := lib.RemoveFolderIfNoVideo(""); err != nil {
			t.Errorf("RemoveFolderIfNoVideo(\"\") should be nil, got %v", err)
		}
	})
}

func TestProbePath(t *testing.T) {
	t.Run("creates library, download directory and .ignore", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("ProbePath: %v", err)
		}

		downloadDir := filepath.Join(completed, ".torrents")
		if _, err := os.Stat(downloadDir); err != nil {
			t.Errorf("download directory was not created: %v", err)
		}
		if _, err := os.Stat(filepath.Join(downloadDir, ".ignore")); err != nil {
			t.Errorf(".ignore was not created: %v", err)
		}
	})

	t.Run("does not leave probe files behind", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("ProbePath: %v", err)
		}

		for _, p := range []string{
			filepath.Join(completed, ".aad_link_probe"),
			filepath.Join(completed, ".torrents", ".aad_link_probe"),
		} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("probe file left behind at %s", p)
			}
		}
	})

	t.Run("rejects empty library", func(t *testing.T) {
		lib := NewLibrarian(NewOSFileSystem())
		if err := lib.ProbePath(""); err == nil {
			t.Error("want error for empty path, got nil")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("first call: %v", err)
		}
		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("second call: %v", err)
		}
	})
}

// Multi-season pack: collectVideoFiles flattens the subfolders, so two files with the SAME
// basename would point at the same destination and the second would remove the first's link —
// an episode would vanish from the library. The relative path disambiguates.
func TestOrganizeBatchSameBasenameInSubfoldersKeepsBothLinks(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "Season 1", "[Sub] Anime - 01 [1080p].mkv"), "s1e1")
	writeFile(t, filepath.Join(dataDir, "Season 2", "[Sub] Anime - 01 [1080p].mkv"), "s2e1")

	lib := NewLibrarian(NewOSFileSystem())
	created, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		IsBatch:        true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %v, want 2 links", created)
	}
	if created[0] == created[1] {
		t.Fatalf("both files landed on the same library path: %v", created)
	}
	for path, want := range map[string]string{
		filepath.Join(completed, "Anime", "[Sub] Anime - 01 [1080p].mkv"):            "s1e1",
		filepath.Join(completed, "Anime", "Season 2 - [Sub] Anime - 01 [1080p].mkv"): "s2e1",
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected link %s: %v", path, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

// Season >= 2 pack with continuous numbering (files 13-24 for a 12-episode entry):
// the file number is absolute, the AniList ENTRY number starts at 1.
func TestOrganizeBatchContinuousNumberingMapsToEntryNumbers(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime S2 - 13 [1080p].mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime S2 - 14 [1080p].mkv"), "b")

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  2,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize: %v", err)
	}
	for _, name := range []string{"Anime Season 2 - E01.mkv", "Anime Season 2 - E02.mkv"} {
		if _, err := os.Stat(filepath.Join(completed, "Anime Season 2", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

// The translation only holds with unequivocal evidence. An extra numbered above the total (13
// in a 01-12 pack) keeps the minimum at 1: nothing is shifted.
func TestOrganizeBatchExtraAboveTotalDoesNotShift(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 01 [1080p].mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 02 [1080p].mkv"), "b")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime - 13 [1080p].mkv"), "c")

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime",
		CompletedPath:  completed,
		TotalEpisodes:  2,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize: %v", err)
	}
	for _, name := range []string{"Anime - E01.mkv", "Anime - E02.mkv", "Anime - E13.mkv"} {
		if _, err := os.Stat(filepath.Join(completed, "Anime", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

// Incomplete pack (fewer files than the entry total) cannot be claimed to start at
// episode 1 of the season: it stays as-is.
func TestOrganizeBatchIncompletePackDoesNotShift(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "batchid")
	completed := filepath.Join(tmp, "completed")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime S2 - 23 [1080p].mkv"), "a")
	writeFile(t, filepath.Join(dataDir, "[Sub] Anime S2 - 24 [1080p].mkv"), "b")

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Anime Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize: %v", err)
	}
	for _, name := range []string{"Anime Season 2 - E23.mkv", "Anime Season 2 - E24.mkv"} {
		if _, err := os.Stat(filepath.Join(completed, "Anime Season 2", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

func TestPackEpisodeOffset(t *testing.T) {
	cases := []struct {
		name    string
		numbers []int
		total   int
		want    int
	}{
		{"complete season 2 pack with continuous numbering", []int{13, 14, 15}, 3, 12},
		{"complete pack numbered from 1", []int{1, 2, 3}, 3, 0},
		{"extra above the total does not shift", []int{1, 2, 13}, 2, 0},
		{"incomplete pack does not shift", []int{23, 24}, 12, 0},
		{"unknown total does not shift", []int{13, 14}, 0, 0},
		{"no numbered files", nil, 12, 0},
	}
	for _, c := range cases {
		if got := packEpisodeOffset(c.numbers, c.total); got != c.want {
			t.Errorf("%s: packEpisodeOffset(%v, %d) = %d, want %d", c.name, c.numbers, c.total, got, c.want)
		}
	}
}

func libraryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && e.Name() != "tvshow.nfo" {
			names = append(names, e.Name())
		}
	}
	return names
}

func writeNumberedSeason(t *testing.T, dataDir, folder, pattern string, from, to int) {
	t.Helper()
	for i := from; i <= to; i++ {
		name := fmt.Sprintf(pattern, i)
		writeFile(t, filepath.Join(dataDir, folder, name), name)
	}
}

// Kaguya EMBER S1+S2 (nyaa.si/view/1355132): S01 files land in the S1 folder, S02 stay out.
func TestOrganizeRestartPackRoutesSeasonFilesToThisEntry(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "ember")
	completed := filepath.Join(tmp, "completed")
	writeNumberedSeason(t, dataDir, "S01", "S01E%02d-Episode.mkv", 1, 12)
	writeNumberedSeason(t, dataDir, "S02", "S02E%02d-Episode.mkv", 1, 12)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Kaguya-sama Love is War",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   0,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S1: %v", err)
	}
	s1dir := filepath.Join(completed, "Kaguya-sama Love is War")
	got := libraryNames(t, s1dir)
	if len(got) != 12 {
		t.Fatalf("S1 library has %d files, want 12: %v", len(got), got)
	}
	for i := 1; i <= 12; i++ {
		want := fmt.Sprintf("Kaguya-sama Love is War - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(s1dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}

	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Kaguya-sama Love is War Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   12,
		AnimeSeason:    intPtr(2),
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S2: %v", err)
	}
	s2dir := filepath.Join(completed, "Kaguya-sama Love is War Season 2")
	if got := libraryNames(t, s2dir); len(got) != 12 {
		t.Fatalf("S2 library has %d files, want 12: %v", len(got), got)
	}
	for i := 1; i <= 12; i++ {
		want := fmt.Sprintf("Kaguya-sama Love is War Season 2 - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(s2dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// Mushoku 01-23 under cour 1 (offset 0, total 11) keeps 01-11; cour 2 (offset 11, total 12)
// maps 12-23 onto E01-E12.
func TestOrganizeContinuousPackSlicesBySeriesOffset(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "mushoku")
	completed := filepath.Join(tmp, "completed")
	for i := 1; i <= 23; i++ {
		name := fmt.Sprintf("[MTBB] Mushoku Tensei - %02d.mkv", i)
		writeFile(t, filepath.Join(dataDir, name), name)
	}

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei",
		CompletedPath:  completed,
		TotalEpisodes:  11,
		SeriesOffset:   0,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize cour 1: %v", err)
	}
	c1 := filepath.Join(completed, "Mushoku Tensei")
	if got := libraryNames(t, c1); len(got) != 11 {
		t.Fatalf("cour 1 library has %d files, want 11: %v", len(got), got)
	}

	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei Cour 2",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   11,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize cour 2: %v", err)
	}
	c2 := filepath.Join(completed, "Mushoku Tensei Cour 2")
	if got := libraryNames(t, c2); len(got) != 12 {
		t.Fatalf("cour 2 library has %d files, want 12: %v", len(got), got)
	}
	for i := 1; i <= 12; i++ {
		want := fmt.Sprintf("Mushoku Tensei Cour 2 - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(c2, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// Anime Time S1+S2 (nyaa.si/view/2169474): S01E17.5 (Eris) and S02E00 (Guardian Fitz)
// stay out of the pack span (#113) and still land in the matching season folder.
func TestOrganizeRestartPackHardlinksMushokuExtras(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "animetime")
	completed := filepath.Join(tmp, "completed")
	writeNumberedSeason(t, dataDir, "Season 1", "Mushoku Tensei Jobless Reincarnation - S01E%02d - Title.mkv", 1, 23)
	const eris = "Mushoku Tensei Jobless Reincarnation - S01E17.5 - Eris The Goblin Slayer.mkv"
	writeFile(t, filepath.Join(dataDir, "Season 1", eris), eris)
	writeNumberedSeason(t, dataDir, "Season 2", "Mushoku Tensei Jobless Reincarnation - S02E%02d - Title.mkv", 1, 24)
	const fitz = "Mushoku Tensei Jobless Reincarnation - S02E00 - Guardian Fitz.mkv"
	writeFile(t, filepath.Join(dataDir, "Season 2", fitz), fitz)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei",
		CompletedPath:  completed,
		TotalEpisodes:  23,
		SeriesOffset:   0,
		AnimeSeason:    intPtr(1),
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S1: %v", err)
	}
	s1 := filepath.Join(completed, "Mushoku Tensei")
	if _, err := os.Stat(filepath.Join(s1, eris)); err != nil {
		t.Fatalf("S1 is missing the Eris OVA hardlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s1, fitz)); err == nil {
		t.Fatal("S2 episode 0 landed in the S1 folder")
	}
	for i := 1; i <= 23; i++ {
		want := fmt.Sprintf("Mushoku Tensei - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(s1, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}

	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  24,
		SeriesOffset:   23,
		AnimeSeason:    intPtr(2),
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S2: %v", err)
	}
	s2 := filepath.Join(completed, "Mushoku Tensei Season 2")
	if _, err := os.Stat(filepath.Join(s2, fitz)); err != nil {
		t.Fatalf("S2 is missing the episode 0 hardlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s2, eris)); err == nil {
		t.Fatal("Eris OVA landed in the S2 folder")
	}
	if _, err := os.Stat(filepath.Join(s2, "Mushoku Tensei Season 2 - E00.mkv")); err == nil {
		t.Fatal("episode 0 was renamed to E00; the raw name must stay")
	}
	for i := 1; i <= 24; i++ {
		want := fmt.Sprintf("Mushoku Tensei Season 2 - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(s2, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// EMBER names the Eris OVA S01E24 while the TV files are plain 01-23, so the extra
// sits in its own season group. PackFileRange drops it from the span; Organize still
// hardlinks it under the raw name — not as E24 of an 11-episode cour.
func TestOrganizeContinuousPackHardlinksMushokuOVA(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "mushoku")
	completed := filepath.Join(tmp, "completed")
	for i := 1; i <= 23; i++ {
		name := fmt.Sprintf("[MTBB] Mushoku Tensei - %02d.mkv", i)
		writeFile(t, filepath.Join(dataDir, name), name)
	}
	const ova = "[EMBER] Mushoku Tensei S01E24 [OVA]-Eris the Goblin Slayer.mkv"
	writeFile(t, filepath.Join(dataDir, ova), ova)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei",
		CompletedPath:  completed,
		TotalEpisodes:  11,
		SeriesOffset:   0,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize cour 1: %v", err)
	}
	c1 := filepath.Join(completed, "Mushoku Tensei")
	if _, err := os.Stat(filepath.Join(c1, ova)); err != nil {
		t.Fatalf("cour 1 is missing the OVA hardlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c1, "Mushoku Tensei - E24.mkv")); err == nil {
		t.Fatal("OVA was renamed to E24; the raw name must stay")
	}
	for i := 1; i <= 11; i++ {
		want := fmt.Sprintf("Mushoku Tensei - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(c1, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// Erai-raws S2 Part 1 numbers Guardian Fitz as - 00 next to 01-12. The 00 stays
// out of the span and is hardlinked with the raw name into the Part 1 folder.
func TestOrganizePackHardlinksEpisodeZero(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "erai")
	completed := filepath.Join(tmp, "completed")
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("[Erai-raws] Mushoku Tensei II - %02d.mkv", i)
		writeFile(t, filepath.Join(dataDir, name), name)
	}
	const ep0 = "[Erai-raws] Mushoku Tensei II - 00.mkv"
	writeFile(t, filepath.Join(dataDir, ep0), ep0)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   0,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize: %v", err)
	}
	dir := filepath.Join(completed, "Mushoku Tensei Season 2")
	if _, err := os.Stat(filepath.Join(dir, ep0)); err != nil {
		t.Fatalf("missing episode 0 hardlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Mushoku Tensei Season 2 - E00.mkv")); err == nil {
		t.Fatal("episode 0 was renamed to E00; the raw name must stay")
	}
	for i := 1; i <= 12; i++ {
		want := fmt.Sprintf("Mushoku Tensei Season 2 - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// S02E00 next to un-tagged 01-12 files is an extras-only season group, same
// shape as the EMBER S01E24 OVA. It still has to land in the library.
func TestOrganizePackHardlinksSeasonTaggedEpisodeZero(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "erai")
	completed := filepath.Join(tmp, "completed")
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("[Group] Mushoku Tensei - %02d.mkv", i)
		writeFile(t, filepath.Join(dataDir, name), name)
	}
	const ep0 = "[Group] Mushoku Tensei - S02E00 - Guardian Fitz.mkv"
	writeFile(t, filepath.Join(dataDir, ep0), ep0)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Mushoku Tensei Season 2",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   0,
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize: %v", err)
	}
	dir := filepath.Join(completed, "Mushoku Tensei Season 2")
	if _, err := os.Stat(filepath.Join(dir, ep0)); err != nil {
		t.Fatalf("missing episode 0 hardlink: %v", err)
	}
	for i := 1; i <= 12; i++ {
		want := fmt.Sprintf("Mushoku Tensei Season 2 - E%02d.mkv", i)
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// AoT EMBER S1+S2+S3+S4P1 (nyaa.si/view/1421679): S3 (offset 37, total 12) takes S03E01-12;
// Final Season Part 2 is a later AniList entry (offset 49, total 10) and takes S03E13-22.
func TestOrganizeRestartPackSplitsCoursInsideOneSeason(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "save", "aot")
	completed := filepath.Join(tmp, "completed")
	writeNumberedSeason(t, dataDir, "S01", "S01E%02d-Episode.mkv", 1, 25)
	writeNumberedSeason(t, dataDir, "S02", "S02E%02d-Episode.mkv", 1, 12)
	writeNumberedSeason(t, dataDir, "S03P01", "S03E%02d-Episode.mkv", 1, 12)
	writeNumberedSeason(t, dataDir, "S03P02", "S03E%02d-Episode.mkv", 13, 22)
	writeNumberedSeason(t, dataDir, "S04P01", "S04E%02d-Episode.mkv", 1, 16)

	lib := NewLibrarian(NewOSFileSystem())
	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Shingeki no Kyojin Season 3",
		CompletedPath:  completed,
		TotalEpisodes:  12,
		SeriesOffset:   37,
		AnimeSeason:    intPtr(3),
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S3: %v", err)
	}
	s3 := filepath.Join(completed, "Shingeki no Kyojin Season 3")
	if got := libraryNames(t, s3); len(got) != 12 {
		t.Fatalf("S3 library has %d files, want 12: %v", len(got), got)
	}
	if _, err := os.Stat(filepath.Join(s3, "Shingeki no Kyojin Season 3 - E01.mkv")); err != nil {
		t.Errorf("S3 E01: %v", err)
	}

	if _, err := lib.Organize(OrganizeRequest{
		TorrentDataDir: dataDir,
		AnimeName:      "Shingeki no Kyojin Season 3 Part 2",
		CompletedPath:  completed,
		TotalEpisodes:  10,
		SeriesOffset:   49,
		AnimeSeason:    intPtr(3),
		IsBatch:        true,
		RenameJellyfin: true,
	}); err != nil {
		t.Fatalf("Organize S3P2: %v", err)
	}
	p2 := filepath.Join(completed, "Shingeki no Kyojin Season 3 Part 2")
	if got := libraryNames(t, p2); len(got) != 10 {
		t.Fatalf("S3P2 library has %d files, want 10: %v", len(got), got)
	}
	if _, err := os.Stat(filepath.Join(p2, "Shingeki no Kyojin Season 3 Part 2 - E01.mkv")); err != nil {
		t.Errorf("S3P2 E01 from S03E13: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p2, "Shingeki no Kyojin Season 3 Part 2 - E10.mkv")); err != nil {
		t.Errorf("S3P2 E10 from S03E22: %v", err)
	}
}
