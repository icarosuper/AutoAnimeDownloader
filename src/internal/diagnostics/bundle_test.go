package diagnostics

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfig = `{
  "completed_anime_path": "/home/alice/Animes",
  "anilist_usernames": ["JediahSk"],
  "mal_usernames": ["MalUser"],
  "notifications": {"webhooks": [{"name": "discord", "method": "POST",
    "url": "https://discord.com/api/webhooks/1/tok", "headers": {"Authorization": "Bearer abcdefgh123"},
    "body": "{\"content\":\"hi\"}", "events": []}]}
}`

func bundleDir(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", "/home/alice")
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, configFileName), testConfig)
	writeFile(t, filepath.Join(dir, animeSettingsFileName), `{"154587":{"progress":3}}`)
	return dir
}

func TestBuildRedactsTheConfig(t *testing.T) {
	bundle := Build(bundleDir(t), Options{Titles: true}).Bundle
	for _, leaked := range []string{"JediahSk", "MalUser", "/home/alice", "webhooks/1/tok", "Bearer", "content"} {
		if strings.Contains(bundle, leaked) {
			t.Errorf("bundle leaks %q:\n%s", leaked, bundle)
		}
	}
	for _, want := range []string{`"user1"`, `"user2"`, `"~/Animes"`, `"name": "discord"`, `"url": "<redacted>"`} {
		if !strings.Contains(bundle, want) {
			t.Errorf("bundle misses %q:\n%s", want, bundle)
		}
	}
}

func TestBuildWithTitlesOffHidesAnimeSettingsAndContextIDs(t *testing.T) {
	dir := bundleDir(t)
	writeFile(t, filepath.Join(dir, logFileName), `{"level":"info","anime_id":154587,"message":"x"}`+"\n")

	bundle := Build(dir, Options{Titles: false, Error: "boom", Context: "GET /animes/154587/episodes 500", AnimeID: "154587"}).Bundle

	if !strings.Contains(bundle, "Anime media ID: <title-1>") || !strings.Contains(bundle, `"<title-1>": {`) ||
		!strings.Contains(bundle, `"anime_id":"<title-1>"`) {
		t.Fatalf("media id not hidden consistently:\n%s", bundle)
	}
	if !strings.Contains(bundle, "Request: GET /animes/<title-1>/episodes 500") {
		t.Fatalf("context missing:\n%s", bundle)
	}
}

func TestBuildIncludesThePendingCrash(t *testing.T) {
	dir := bundleDir(t)
	writeFile(t, filepath.Join(dir, pendingCrashName), "panic: boom\n\ngoroutine 1 [running]:\n/home/alice/src/x.go:1\n")
	bundle := Build(dir, Options{Titles: true}).Bundle
	if !strings.Contains(bundle, "## Crash\n\npanic: boom") || strings.Contains(bundle, "/home/alice/src") {
		t.Fatalf("crash section wrong:\n%s", bundle)
	}
}

func TestLastLogLinesFallsBackToTheNewestBackupAfterRotation(t *testing.T) {
	dir := t.TempDir()
	writeGzip(t, filepath.Join(dir, "daemon-2026-08-01T00-00-00.000.log.gz"), "old backup\n")
	writeGzip(t, filepath.Join(dir, "daemon-2026-08-09T00-00-00.000.log.gz"), "backup 1\nbackup 2\n")
	writeFile(t, filepath.Join(dir, logFileName), "active 1\n"+`{"message":"HTTP request"}`+"\n")

	got := lastLogLines(dir, 2)
	if strings.Join(got, "|") != "backup 2|active 1" {
		t.Fatalf("got %v", got)
	}
}

func TestLastLogLinesSkipsTheBackupWhenTheActiveFileIsEnough(t *testing.T) {
	dir := t.TempDir()
	writeGzip(t, filepath.Join(dir, "daemon-2026-08-09T00-00-00.000.log.gz"), "backup\n")
	var active strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&active, "line %d\n", i)
	}
	writeFile(t, filepath.Join(dir, logFileName), active.String())

	if got := lastLogLines(dir, 2); strings.Join(got, "|") != "line 1|line 2" {
		t.Fatalf("got %v", got)
	}
}

func writeGzip(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	gz.Write([]byte(content))
	gz.Close()
}
