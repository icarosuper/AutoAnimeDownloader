package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/api"
	"AutoAnimeDownloader/src/internal/files"
)

// configFields returns the default config in the same shape handleConfigSet uses: the raw JSON
// map. It is the source of truth for the tests below — no hand-written field list, so a new
// files.Config field is picked up for free.
func configFields(t *testing.T) map[string]any {
	t.Helper()
	raw, err := json.Marshal(files.Config{AnilistUsernames: []string{"a"}})
	if err != nil {
		t.Fatalf("failed to serialize config: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("failed to deserialize config: %v", err)
	}
	return fields
}

// The old switch covered 7 keys and answered "unknown config key" for the rest. The passthrough
// has to accept EVERY key the config serializes — this test breaks if someone reintroduces a
// fixed list.
func TestMatchConfigKey_ResolvesEveryConfigKey(t *testing.T) {
	fields := configFields(t)
	if len(fields) < 20 {
		t.Fatalf("expected the full config, got %d keys", len(fields))
	}
	for name := range fields {
		if got, ok := matchConfigKey(fields, name); !ok || got != name {
			t.Errorf("key %q did not resolve (got=%q ok=%v)", name, got, ok)
		}
	}
}

func TestMatchConfigKey_IgnoresCaseAndUnderscore(t *testing.T) {
	fields := configFields(t)
	for _, typed := range []string{"max_search_pages", "maxSearchPages", "MAX_SEARCH_PAGES", "MaxSearchPages"} {
		if got, ok := matchConfigKey(fields, typed); !ok || got != "max_search_pages" {
			t.Errorf("%q should resolve to max_search_pages, got (%q, %v)", typed, got, ok)
		}
	}
	if _, ok := matchConfigKey(fields, "does_not_exist"); ok {
		t.Error("missing key should fail instead of matching something")
	}
}

// A dotted key patches one field of a nested object in place. Before it, changing
// batch_window_seconds meant passing the whole notifications object, and a webhook left out of
// that JSON was erased.
func TestResolveConfigKey_WalksDottedKey(t *testing.T) {
	fields := configFields(t)
	fields["notifications"] = map[string]any{
		"webhooks":             []any{map[string]any{"name": "ntfy"}},
		"batch_window_seconds": float64(60),
	}

	parent, name, err := resolveConfigKey(fields, "Notifications.BatchWindowSeconds")
	if err != nil {
		t.Fatalf("dotted key did not resolve: %v", err)
	}
	if name != "batch_window_seconds" {
		t.Fatalf("resolved to %q, want batch_window_seconds", name)
	}
	parent[name] = parseConfigValue(parent[name], "30")

	notifications := fields["notifications"].(map[string]any)
	if notifications["batch_window_seconds"] != float64(30) {
		t.Errorf("value not written in place: %v", notifications["batch_window_seconds"])
	}
	if len(notifications["webhooks"].([]any)) != 1 {
		t.Error("sibling field lost by the dotted write")
	}

	for _, bad := range []string{"notifications.does_not_exist", "check_interval.minutes", "does_not_exist.x"} {
		if _, _, err := resolveConfigKey(fields, bad); err == nil {
			t.Errorf("%q should fail as an unknown key", bad)
		}
	}
}

func TestParseConfigValue(t *testing.T) {
	tests := []struct {
		name    string
		current any
		value   string
		want    any
	}{
		// The saved value is the type oracle: only a field that is already a list accepts the
		// comma-separated form.
		{"comma-separated list", []any{"a"}, "x, y ,z", []any{"x", "y", "z"}},
		{"list drops empty items", []any{"a"}, "x,,y", []any{"x", "y"}},
		{"explicit JSON list", []any{"a"}, `["x","y"]`, []any{"x", "y"}},
		{"int", float64(5), "12", float64(12)},
		{"bool", false, "true", true},
		{"float", float64(1), "2.5", 2.5},
		// A string with a comma does NOT become a list when the field is not a list — that was
		// the one way the passthrough could corrupt a library path.
		{"string with comma", "", "/home/me/Animes, Series", "/home/me/Animes, Series"},
		{"plain string", "", "/home/me/Animes", "/home/me/Animes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseConfigValue(tt.current, tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseConfigValue(%v, %q) = %#v, want %#v", tt.current, tt.value, got, tt.want)
			}
		})
	}
}

func TestConfigGetPrintsEverySerializedKey(t *testing.T) {
	cfg := files.Config{AnilistUsernames: []string{"a"}}
	fields := configFields(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/config" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: cfg})
	}))
	defer srv.Close()

	stdoutJSON, _, err := runCLI(t, "--endpoint", srv.URL, "--json", "config", "get")
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var printed map[string]any
	if err := json.Unmarshal([]byte(stdoutJSON), &printed); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdoutJSON)
	}
	for name := range fields {
		if _, ok := printed[name]; !ok {
			t.Errorf("JSON missing key %q", name)
		}
	}

	stdoutTable, _, err := runCLI(t, "--endpoint", srv.URL, "config", "get")
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	for name := range fields {
		if !strings.Contains(stdoutTable, name) {
			t.Errorf("table missing key %q", name)
		}
	}
}

// TestNotificationsPreviewSendsTheSavedPreset: the CLI previews a SAVED preset by name — it reads
// the config and posts that preset to the render endpoint, which never fires it.
func TestNotificationsPreviewSendsTheSavedPreset(t *testing.T) {
	cfg := files.Config{Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{
		{Name: "other", URL: "http://other"},
		{Name: "ntfy", URL: "http://ntfy/{{anime_id}}", Method: "POST"},
	}}}

	var posted files.WebhookPreset
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/config":
			_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: cfg})
		case "/api/v1/notifications/webhooks/preview":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decode preset: %v", err)
			}
			_ = json.NewEncoder(w).Encode(api.SuccessResponse{Success: true, Data: map[string]any{
				"method": "POST", "url": "http://ntfy/154587", "headers": map[string]string{}, "body": "",
			}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLI(t, "--endpoint", srv.URL, "notifications", "preview", "ntfy")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if posted.URL != "http://ntfy/{{anime_id}}" {
		t.Errorf("posted the wrong preset: %+v", posted)
	}
	if !strings.Contains(stdout, "http://ntfy/154587") {
		t.Errorf("rendered url missing from output:\n%s", stdout)
	}

	if _, _, err := runCLI(t, "--endpoint", srv.URL, "notifications", "preview", "missing"); err == nil {
		t.Error("an unknown preset name should fail")
	}
}
