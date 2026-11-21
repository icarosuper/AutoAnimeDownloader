package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestReorderInterspersedFlags(t *testing.T) {
	app := newApp()
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "magnet after id",
			in:   []string{"aad", "animes", "add", "21", "--magnet", "magnet:?xt=urn:btih:abc"},
			want: []string{"aad", "animes", "add", "--magnet", "magnet:?xt=urn:btih:abc", "21"},
		},
		{
			name: "magnet before id is unchanged",
			in:   []string{"aad", "animes", "add", "--magnet", "magnet:?xt=urn:btih:abc", "21"},
			want: []string{"aad", "animes", "add", "--magnet", "magnet:?xt=urn:btih:abc", "21"},
		},
		{
			name: "global json after the id moves to the front",
			in:   []string{"aad", "animes", "add", "21", "--json", "--magnet", "magnet:?xt=urn:btih:abc"},
			want: []string{"aad", "--json", "animes", "add", "--magnet", "magnet:?xt=urn:btih:abc", "21"},
		},
		{
			name: "query after id",
			in:   []string{"aad", "animes", "candidates", "21", "--query", "foo"},
			want: []string{"aad", "animes", "candidates", "--query", "foo", "21"},
		},
		{
			name: "double-dash stops reordering",
			in:   []string{"aad", "animes", "search", "--", "--unreleased"},
			want: []string{"aad", "animes", "search", "--", "--unreleased"},
		},
		{
			name: "replace magnet and yes after id",
			in:   []string{"aad", "animes", "replace", "21", "--magnet", "magnet:?xt=urn:btih:abc", "--yes"},
			want: []string{"aad", "animes", "replace", "--magnet", "magnet:?xt=urn:btih:abc", "--yes", "21"},
		},
		{
			name: "help stays with the subcommand",
			in:   []string{"aad", "animes", "add", "--help"},
			want: []string{"aad", "animes", "add", "--help"},
		},
		{
			name: "help after a positional drops the positional",
			in:   []string{"aad", "--json", "animes", "add", "21", "-h"},
			want: []string{"aad", "--json", "animes", "add", "-h"},
		},
		{
			name: "help on the parent command",
			in:   []string{"aad", "animes", "--help"},
			want: []string{"aad", "animes", "--help"},
		},
		{
			name: "help after double-dash is a positional",
			in:   []string{"aad", "animes", "search", "--", "--help"},
			want: []string{"aad", "animes", "search", "--", "--help"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderInterspersedFlags(app, tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A nested command's --help prints ITS help, not the parent's (the reorder used to move --help to
// the front, so `animes add --help` printed a truncated help of `animes`).
func TestNestedHelpPrintsTheSubcommandHelp(t *testing.T) {
	for _, args := range [][]string{
		{"animes", "add", "--help"},
		{"animes", "add", "21", "-h"},
	} {
		stdout, _, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(stdout, "animes add - Add a standalone anime") || !strings.Contains(stdout, "--magnet") {
			t.Errorf("%v printed:\n%s", args, stdout)
		}
	}
	stdout, _, err := runCLI(t, "animes", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "COMMANDS:") || !strings.Contains(stdout, "--account") {
		t.Errorf("animes --help is missing its subcommands or options:\n%s", stdout)
	}
}
