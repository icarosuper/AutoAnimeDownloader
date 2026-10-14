package diagnostics

import (
	"strings"
	"testing"
)

func testRedactor(titles bool) *redactor {
	return newRedactor(titles, "/home/alice", []string{"JediahSk", "D4NIDx"},
		[]string{"https://discord.com/api/webhooks/123/secret-token", "Bearer abcdefgh123"})
}

func mustLine(t *testing.T, r *redactor, line string) string {
	t.Helper()
	out, ok := r.logLine(line)
	if !ok {
		t.Fatalf("line was dropped: %s", line)
	}
	return out
}

func TestLogLineDropsHTTPRequestLines(t *testing.T) {
	r := testRedactor(true)
	line := `{"level":"info","method":"GET","path":"/api/v1/status","remote_addr":"192.168.0.100:5000","status":200,"time":"2026-08-09T22:21:20-03:00","message":"HTTP request"}`
	if _, ok := r.logLine(line); ok {
		t.Fatal("HTTP request line should be dropped")
	}
}

func TestLogLineDropsRemoteAddrKey(t *testing.T) {
	out := mustLine(t, testRedactor(true), `{"level":"info","remote_addr":"192.168.0.100:5000","message":"x"}`)
	if strings.Contains(out, "remote_addr") || strings.Contains(out, "192.168") {
		t.Fatalf("remote_addr survived: %s", out)
	}
}

func TestLogLinePseudonymizesUsernamesInKeysAndText(t *testing.T) {
	r := testRedactor(true)
	out := mustLine(t, r, `{"level":"info","usernames":["JediahSk","D4NIDx"],"animes_found":21,"message":"Successfully fetched animes from Anilist"}`)
	if !strings.Contains(out, `"usernames":["user1","user2"]`) {
		t.Fatalf("usernames not pseudonymized: %s", out)
	}
	out = mustLine(t, r, `{"level":"error","error":"user jediahsk not found","message":"x"}`)
	if !strings.Contains(out, "user user1 not found") {
		t.Fatalf("username inside error text survived: %s", out)
	}
	// An account no longer in the config gets the next pseudonym when a user key shows it.
	out = mustLine(t, r, `{"level":"info","mal_user":"OldAccount","message":"x"}`)
	if !strings.Contains(out, `"mal_user":"user3"`) {
		t.Fatalf("unknown username not pseudonymized: %s", out)
	}
}

func TestUsernameReplacementMatchesWholeWordsOnly(t *testing.T) {
	r := newRedactor(true, "", []string{"test"}, nil)
	if got := r.scrub("latest test"); got != "latest user1" {
		t.Fatalf("got %q", got)
	}
}

func TestLogLineReplacesWebhookURLInsideError(t *testing.T) {
	out := mustLine(t, testRedactor(true), `{"level":"error","error":"Post \"https://discord.com/api/webhooks/123/secret-token\": dial tcp: timeout","webhook":"discord","message":"Webhook request failed"}`)
	if strings.Contains(out, "secret-token") || !strings.Contains(out, "<webhook-1>") {
		t.Fatalf("webhook URL survived: %s", out)
	}
}

func TestLogLineReplacesURLsOfUnknownHosts(t *testing.T) {
	// A rotated webhook: the current config doesn't know it anymore.
	out := mustLine(t, testRedactor(true), `{"level":"error","error":"Post \"https://ntfy.sh/old-topic-xyz\": EOF","message":"Webhook request failed"}`)
	if strings.Contains(out, "old-topic") || !strings.Contains(out, "<url>") {
		t.Fatalf("unknown URL survived: %s", out)
	}
	out = mustLine(t, testRedactor(true), `{"level":"debug","url":"https://graphql.anilist.co","message":"Sending Anilist request"}`)
	if !strings.Contains(out, "https://graphql.anilist.co") {
		t.Fatalf("known host was replaced: %s", out)
	}
}

func TestLogLineReplacesHomeDirectory(t *testing.T) {
	out := mustLine(t, testRedactor(true), `{"level":"info","download_path":"/home/alice/Animes/.torrents","message":"x"}`)
	if !strings.Contains(out, `"download_path":"~/Animes/.torrents"`) {
		t.Fatalf("home survived: %s", out)
	}
}

func TestHomeIsReplacedBeforeAMatchingUsername(t *testing.T) {
	r := newRedactor(true, "/home/alice", []string{"alice"}, nil)
	if got := r.scrub("/home/alice/Animes"); got != "~/Animes" {
		t.Fatalf("got %q", got)
	}
}

func TestTitlesOffKeepsTheSameTokenAcrossLines(t *testing.T) {
	r := testRedactor(false)
	a := mustLine(t, r, `{"level":"info","anime":"Sparks of Tomorrow","message":"Processing anime episodes"}`)
	b := mustLine(t, r, `{"level":"info","anime":"Chainsmoker Cat","message":"Processing anime episodes"}`)
	c := mustLine(t, r, `{"level":"info","anime":"Sparks of Tomorrow","anime_id":154587,"message":"Processing anime episodes"}`)
	if strings.Contains(a+b+c, "Sparks") || strings.Contains(c, "154587") {
		t.Fatalf("title survived:\n%s\n%s\n%s", a, b, c)
	}
	if !strings.Contains(a, `"anime":"<title-1>"`) || !strings.Contains(b, `"anime":"<title-2>"`) || !strings.Contains(c, `"anime":"<title-1>"`) {
		t.Fatalf("tokens not stable:\n%s\n%s\n%s", a, b, c)
	}
	if !strings.Contains(c, `"anime_id":"<title-3>"`) {
		t.Fatalf("anime id not hidden: %s", c)
	}
}

func TestTitlesOffHidesNyaaURLsInFreeText(t *testing.T) {
	out := mustLine(t, testRedactor(false), `{"level":"error","error":"Get \"https://nyaa.si/?q=Chainsmoker+Cat\": timeout","message":"x"}`)
	if strings.Contains(out, "Chainsmoker") || !strings.Contains(out, "<title-1>") {
		t.Fatalf("nyaa URL survived: %s", out)
	}
}

func TestTitlesOnKeepsTitles(t *testing.T) {
	out := mustLine(t, testRedactor(true), `{"level":"info","anime":"Sparks of Tomorrow","hash":"4cc95a4e","message":"x"}`)
	if !strings.Contains(out, "Sparks of Tomorrow") || !strings.Contains(out, "4cc95a4e") {
		t.Fatalf("titles hidden with titles on: %s", out)
	}
}

func TestLogLineKeepsFieldOrderAndDuplicateKeys(t *testing.T) {
	// anilist.go logs an explicit "message" field next to zerolog's own.
	line := `{"level":"warn","graphql_status":400,"message":"Invalid token","time":"t","message":"Anilist returned an error inside a 200 response"}`
	out := mustLine(t, testRedactor(true), line)
	if out != line {
		t.Fatalf("line changed:\n got %s\nwant %s", out, line)
	}
}

func TestLogLineScrubsNonJSONLines(t *testing.T) {
	out := mustLine(t, testRedactor(true), "panic: open /home/alice/x: permission denied")
	if out != "panic: open ~/x: permission denied" {
		t.Fatalf("got %q", out)
	}
}

func TestWebhookSecretsSkipPrefixesWithoutHost(t *testing.T) {
	var cfg rawConfig
	cfg.Notifications.Webhooks = []rawWebhook{{URL: "https://{{host}}/x"}, {URL: "https://ntfy.sh/{{topic}}"}}
	got := webhookSecrets(cfg)
	if len(got) != 1 || got[0] != "https://ntfy.sh/" {
		t.Fatalf("got %q", got)
	}
}
