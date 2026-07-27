package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// migrationFM e um FileManagerInterface que guarda o config em memoria, para que o teste
// possa ver o que MigrateSavePath persistiu.
type migrationFM struct {
	mockFileManagerForEpisodes
	configs *files.Config
	saves   int
}

func (m *migrationFM) LoadConfigs() (*files.Config, error) { return m.configs, nil }
func (m *migrationFM) SaveConfigs(c *files.Config) error {
	m.configs = c
	m.saves++
	return nil
}

// makeTorrentDataDirAt cria <root>/<id> com um arquivo dentro e devolve o caminho.
func makeTorrentDataDirAt(t *testing.T, root, id string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "episode.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	return dir
}

func TestMigrateSavePath_MovesTorrentDirs(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dirA := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")
	dirB := makeTorrentDataDirAt(t, oldSave, "bbbbbbbbbbbbbbbbbbbbbb")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dirA)
	backend.AddCompleted("89abcdef0123456789abcdef0123456789abcdef", dirB)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	dest := filepath.Join(completed, ".autoAnimeDownloader")
	for _, id := range []string{"aaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbb"} {
		moved := filepath.Join(dest, id, "episode.mkv")
		if _, err := os.Stat(moved); err != nil {
			t.Errorf("torrent %s nao foi movido: %v", id, err)
		}
	}
	if fm.configs.SavePath != "" {
		t.Errorf("save_path deveria ter sido zerado, veio %q", fm.configs.SavePath)
	}
	// A sessao temporaria tem que ser aberta no caminho ANTIGO: e de la que os
	// DataDirs sao lidos.
	if calls := backend.EnsureCalls(); len(calls) != 1 || calls[0] != oldSave {
		t.Errorf("EnsureCalls() = %v, quero [%s]", calls, oldSave)
	}
}

// PreservesHardlinks e o teste que sustenta a tese "nada rebaixa": rename no mesmo volume
// preserva o inode, entao o hardlink ja criado na biblioteca continua apontando para os
// mesmos bytes depois da migracao. Usa OSFileSystem sobre t.TempDir() de proposito — um
// FileSystem falso nao tem inode.
func TestMigrateSavePath_PreservesHardlinks(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	src := filepath.Join(dir, "episode.mkv")
	libDir := filepath.Join(completed, "My Anime")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	libLink := filepath.Join(libDir, "My Anime - E05.mkv")
	if err := os.Link(src, libLink); err != nil {
		t.Fatalf("link: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	moved := filepath.Join(completed, ".autoAnimeDownloader", "aaaaaaaaaaaaaaaaaaaaaa", "episode.mkv")
	movedInfo, err := os.Stat(moved)
	if err != nil {
		t.Fatalf("stat arquivo movido: %v", err)
	}
	linkInfo, err := os.Stat(libLink)
	if err != nil {
		t.Fatalf("stat hardlink da biblioteca: %v", err)
	}
	if !os.SameFile(movedInfo, linkInfo) {
		t.Error("o hardlink da biblioteca deixou de apontar para o arquivo do torrent")
	}
}

func TestMigrateSavePath_Idempotent(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	fsys := files.NewOSFileSystem()
	if err := MigrateSavePath(fsys, fm, backend); err != nil {
		t.Fatalf("primeira migracao: %v", err)
	}
	savesDepoisDaPrimeira := fm.saves

	if err := MigrateSavePath(fsys, fm, backend); err != nil {
		t.Fatalf("segunda migracao: %v", err)
	}
	if fm.saves != savesDepoisDaPrimeira {
		t.Errorf("a segunda migracao deveria ser no-op, mas salvou o config de novo")
	}
}

// SkipsAncestorOfDest cobre o layout default do Docker, onde a biblioteca fica DENTRO do
// save path: SAVE_PATH=/app/downloads, COMPLETED_ANIME_PATH=/app/downloads/completed.
func TestMigrateSavePath_SkipsAncestorOfDest(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(oldSave, "completed")
	if err := os.MkdirAll(completed, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	// Um DataDir patologico que e ancestral do destino: nunca pode ser movido.
	backend.AddCompleted("89abcdef0123456789abcdef0123456789abcdef", oldSave)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err != nil {
		t.Fatalf("MigrateSavePath: %v", err)
	}

	moved := filepath.Join(completed, ".autoAnimeDownloader", "aaaaaaaaaaaaaaaaaaaaaa", "episode.mkv")
	if _, err := os.Stat(moved); err != nil {
		t.Errorf("o torrent normal deveria ter sido movido: %v", err)
	}
	if _, err := os.Stat(oldSave); err != nil {
		t.Errorf("o save path nao deveria ter sido movido para dentro de si mesmo: %v", err)
	}
}

func TestMigrateSavePath_AbortsOnFailure(t *testing.T) {
	root := t.TempDir()
	oldSave := filepath.Join(root, "downloads")
	completed := filepath.Join(root, "library")
	dir := makeTorrentDataDirAt(t, oldSave, "aaaaaaaaaaaaaaaaaaaaaa")

	// Destino ocupado por um ARQUIVO: MkdirAll do diretorio de download falha.
	if err := os.MkdirAll(completed, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(completed, ".autoAnimeDownloader"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted("0123456789abcdef0123456789abcdef01234567", dir)
	fm := &migrationFM{configs: &files.Config{SavePath: oldSave, CompletedAnimePath: completed}}

	if err := MigrateSavePath(files.NewOSFileSystem(), fm, backend); err == nil {
		t.Fatal("quero erro, veio nil")
	}
	// A vedacao critica: com o save_path ainda no config, a proxima tentativa migra de
	// novo. Zera-lo aqui faria o daemon apontar para o caminho novo com os dados no
	// antigo, e rebaixar tudo em silencio.
	if fm.configs.SavePath != oldSave {
		t.Errorf("save_path deveria continuar %q, veio %q", oldSave, fm.configs.SavePath)
	}
}

func TestMigrateSavePath_NoOpWhenNothingToDo(t *testing.T) {
	fsys := files.NewOSFileSystem()

	t.Run("save_path vazio", func(t *testing.T) {
		fm := &migrationFM{configs: &files.Config{CompletedAnimePath: t.TempDir()}}
		if err := MigrateSavePath(fsys, fm, torrents.NewFakeBackend()); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.saves != 0 {
			t.Error("nao deveria ter salvo o config")
		}
	})

	t.Run("biblioteca vazia (config incompleta)", func(t *testing.T) {
		fm := &migrationFM{configs: &files.Config{SavePath: t.TempDir()}}
		if err := MigrateSavePath(fsys, fm, torrents.NewFakeBackend()); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.saves != 0 {
			t.Error("nao deveria ter salvo o config")
		}
	})

	t.Run("save_path ja igual ao derivado: so limpa o campo", func(t *testing.T) {
		completed := t.TempDir()
		fm := &migrationFM{configs: &files.Config{
			SavePath:           filepath.Join(completed, ".autoAnimeDownloader"),
			CompletedAnimePath: completed,
		}}
		backend := torrents.NewFakeBackend()
		if err := MigrateSavePath(fsys, fm, backend); err != nil {
			t.Fatalf("MigrateSavePath: %v", err)
		}
		if fm.configs.SavePath != "" {
			t.Errorf("save_path deveria ter sido zerado, veio %q", fm.configs.SavePath)
		}
		if len(backend.EnsureCalls()) != 0 {
			t.Error("nao deveria ter aberto sessao nenhuma")
		}
	})
}
