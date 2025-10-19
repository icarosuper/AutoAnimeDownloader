package tests

import (
	"AutoAnimeDownloader/modules/files"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func withTempHome(t *testing.T, fn func(tmp string)) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "aad_test_home_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	// set HOME or APPDATA depending on OS
	if runtime.GOOS == "windows" {
		orig := os.Getenv("APPDATA")
		_ = os.Setenv("APPDATA", tmp)
		defer os.Setenv("APPDATA", orig)
	} else {
		orig := os.Getenv("HOME")
		_ = os.Setenv("HOME", tmp)
		defer os.Setenv("HOME", orig)
	}

	fn(tmp)
}

func TestSaveLoadEpisodesAndDelete(t *testing.T) {
	withTempHome(t, func(tmp string) {
		// initially no episodes
		eps := files.LoadSavedEpisodes()
		if len(eps) != 0 {
			t.Fatalf("expected 0 episodes, got %d", len(eps))
		}

		// save some episodes
		toSave := []files.EpisodeStruct{
			{EpisodeID: 1, EpisodeHash: "h1", EpisodeName: "Name1"},
			{EpisodeID: 2, EpisodeHash: "h2", EpisodeName: ""},
		}
		files.SaveEpisodesToFile(toSave)

		loaded := files.LoadSavedEpisodes()
		if len(loaded) != 2 {
			t.Fatalf("expected 2 episodes after save, got %d", len(loaded))
		}

		// delete one
		files.DeleteEpisodesFromFile([]int{1})
		afterDel := files.LoadSavedEpisodes()
		if len(afterDel) != 1 {
			t.Fatalf("expected 1 episode after delete, got %d", len(afterDel))
		}
		if afterDel[0].EpisodeID != 2 {
			t.Fatalf("expected remaining episode id 2, got %d", afterDel[0].EpisodeID)
		}
	})
}

func TestDeleteEpisodesFromFile_Noop(t *testing.T) {
	withTempHome(t, func(tmp string) {
		// create file with one episode
		files.SaveEpisodesToFile([]files.EpisodeStruct{{EpisodeID: 10, EpisodeHash: "hh"}})
		// delete non-existing id -> should be noop and not panic
		files.DeleteEpisodesFromFile([]int{999})
		loaded := files.LoadSavedEpisodes()
		if len(loaded) != 1 || loaded[0].EpisodeID != 10 {
			t.Fatalf("expected original episode to remain, got: %#v", loaded)
		}
	})
}

func TestDeleteEmptyFolders(t *testing.T) {
	withTempHome(t, func(tmp string) {
		saveRoot := filepath.Join(tmp, "save-path")
		if err := os.MkdirAll(saveRoot, 0755); err != nil {
			t.Fatalf("mkdir save root: %v", err)
		}

		// create empty folder and non-empty folder
		empty := filepath.Join(saveRoot, "empty")
		nonEmpty := filepath.Join(saveRoot, "full")
		if err := os.MkdirAll(empty, 0755); err != nil {
			t.Fatalf("mkdir empty: %v", err)
		}
		if err := os.MkdirAll(nonEmpty, 0755); err != nil {
			t.Fatalf("mkdir nonEmpty: %v", err)
		}
		// create file inside nonEmpty
		f, err := os.Create(filepath.Join(nonEmpty, "f.txt"))
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		_ = f.Close()

		cfg := files.Config{SavePath: saveRoot}
		files.DeleteEmptyFolders(cfg)

		// empty should be removed
		if _, err := os.Stat(empty); !os.IsNotExist(err) {
			t.Fatalf("expected empty folder removed, stat err: %v", err)
		}
		// nonEmpty should remain
		if _, err := os.Stat(nonEmpty); err != nil {
			t.Fatalf("expected non-empty folder to remain, stat err: %v", err)
		}
	})
}

func TestLoadSaveConfigs_Defaults(t *testing.T) {
	withTempHome(t, func(tmp string) {
		cfg := files.LoadConfigs()
		// defaults from implementation
		if cfg.CheckInterval != 10 {
			t.Fatalf("expected CheckInterval 10, got %d", cfg.CheckInterval)
		}
		if cfg.QBittorrentUrl != "http://127.0.0.1:8080" {
			t.Fatalf("unexpected default QBittorrentUrl: %s", cfg.QBittorrentUrl)
		}

		// modify and save
		cfg.CheckInterval = 42
		files.SaveConfigs(cfg)

		// load again and verify saved value
		cfg2 := files.LoadConfigs()
		if cfg2.CheckInterval != 42 {
			t.Fatalf("expected CheckInterval 42 after save, got %d", cfg2.CheckInterval)
		}
	})
}

func FilesModuleParsesInvalidName() {

}
