package files

import (
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

// O marcador de season TEM de sobreviver: cada season e uma entrada propria na AniList, com
// id e capa proprios. Apagar o marcador jogava Season 1/2/3 na mesma pasta e o Jellyfin
// casava tudo com a season 1 (decisions.md #45).
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
			t.Errorf("%q e %q caem na mesma pasta %q", other, in, got)
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
		t.Errorf("nfo sem uniqueid da anilist:\n%s", data)
	}

	// Nao sobrescreve um nfo existente (match ajustado a mao pelo usuario).
	if err := os.WriteFile(nfo, []byte("manual"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Organize(req); err != nil {
		t.Fatalf("Organize (2a vez): %v", err)
	}
	if data, _ := os.ReadFile(nfo); string(data) != "manual" {
		t.Errorf("nfo existente foi sobrescrito: %s", data)
	}

	// Sem AnimeID nao escreve nada.
	req.AnimeName = "Outro Anime"
	req.AnimeID = 0
	if _, err := lib.Organize(req); err != nil {
		t.Fatalf("Organize (sem id): %v", err)
	}
	if _, err := os.Stat(filepath.Join(completed, "Outro Anime", "tvshow.nfo")); err == nil {
		t.Error("escreveu tvshow.nfo sem AnimeID")
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
		// Segundo episodio do mesmo anime: nao pode duplicar trabalho nem falhar.
		{AnimeID: 194829, AnimeName: "My Anime", LibraryPaths: []string{filepath.Join(existing, "My Anime - E02.mkv")}},
		// Pasta que sumiu do disco.
		{AnimeID: 111, AnimeName: "Deleted Anime", LibraryPaths: []string{filepath.Join(gone, "ep.mkv")}},
		// Sem id e sem paths: ignorados.
		{AnimeID: 0, AnimeName: "No ID", LibraryPaths: []string{filepath.Join(existing, "x.mkv")}},
		{AnimeID: 222, AnimeName: "Not Organized"},
	})

	data, err := os.ReadFile(filepath.Join(existing, "tvshow.nfo"))
	if err != nil {
		t.Fatalf("tvshow.nfo nao foi criado no backfill: %v", err)
	}
	if !strings.Contains(string(data), ">194829<") {
		t.Errorf("nfo com id errado:\n%s", data)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Error("backfill criou a pasta de um anime que ja nao esta em disco")
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

// Com a flag ligada, os arquivos do pack ganham o MESMO nome dos baixados avulsos, na mesma
// pasta do anime. O que nao tem numero legivel (NCOP) e o segundo arquivo que cai no mesmo
// numero ficam com o nome cru — renomear os dois colidiria e um sobrescreveria o outro.
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
		"[Sub] Anime - 02v2 [1080p].mkv", // colisao com E02: nome cru
		"NCOP.mkv",                       // sem numero: nome cru
	} {
		if _, err := os.Stat(filepath.Join(completed, "Anime", name)); err != nil {
			t.Errorf("expected link %s: %v", name, err)
		}
	}
}

// Pack e avulso do mesmo anime dividem UMA pasta (destDir sai de AnimeName, nunca do nome do
// torrent) e, com a flag, o mesmo padrao de nome — entao os episodios se misturam em ordem.
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

func TestProbePath(t *testing.T) {
	t.Run("cria biblioteca, diretorio de download e .ignore", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("ProbePath: %v", err)
		}

		downloadDir := filepath.Join(completed, ".torrents")
		if _, err := os.Stat(downloadDir); err != nil {
			t.Errorf("diretorio de download nao foi criado: %v", err)
		}
		if _, err := os.Stat(filepath.Join(downloadDir, ".ignore")); err != nil {
			t.Errorf(".ignore nao foi criado: %v", err)
		}
	})

	t.Run("nao deixa arquivos de sonda para tras", func(t *testing.T) {
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
				t.Errorf("sobrou arquivo de sonda em %s", p)
			}
		}
	})

	t.Run("rejeita biblioteca vazia", func(t *testing.T) {
		lib := NewLibrarian(NewOSFileSystem())
		if err := lib.ProbePath(""); err == nil {
			t.Error("quero erro para caminho vazio, veio nil")
		}
	})

	t.Run("e idempotente", func(t *testing.T) {
		completed := filepath.Join(t.TempDir(), "library")
		lib := NewLibrarian(NewOSFileSystem())

		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("primeira chamada: %v", err)
		}
		if err := lib.ProbePath(completed); err != nil {
			t.Fatalf("segunda chamada: %v", err)
		}
	})
}
