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
