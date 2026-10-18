package main

import (
	"strings"
	"testing"
)

func TestDestructiveCommandsRequireYes(t *testing.T) {
	commands := []struct {
		args    []string
		message string
	}{
		{[]string{"animes", "remove", "1"}, "pass --yes (optional: --delete-episodes)"},
		{[]string{"animes", "redownload", "1", "2"}, "pass --yes"},
		{[]string{"animes", "delete", "1", "2"}, "pass --yes"},
		{[]string{"animes", "replace", "1", "--magnet", "magnet:?xt=urn:btih:abc"}, "pass --yes"},
		{[]string{"animes", "replace-episode", "1", "2", "--magnet", "magnet:?xt=urn:btih:abc"}, "pass --yes"},
		{[]string{"torrents", "delete", "deadbeef"}, "pass --yes (optional: --keep-data --block)"},
	}

	for _, tt := range commands {
		t.Run(tt.args[0]+" "+tt.args[1], func(t *testing.T) {
			_, _, err := runCLI(t, append([]string{"--endpoint", "http://127.0.0.1:1"}, tt.args...)...)
			if err == nil {
				t.Fatal("expected CONFIRMATION_REQUIRED, got nil")
			}
			code, exit := cliCode(err)
			if code != "CONFIRMATION_REQUIRED" {
				t.Errorf("code = %q (%v), want CONFIRMATION_REQUIRED", code, err)
			}
			if exit != 2 {
				t.Errorf("exit = %d, want 2", exit)
			}
			if err.Error() != tt.message {
				t.Errorf("message = %q, want %q", err.Error(), tt.message)
			}
			if strings.HasPrefix(err.Error(), "pass --yes --") {
				t.Errorf("optional flags must not look required: %q", err.Error())
			}
		})
	}
}
