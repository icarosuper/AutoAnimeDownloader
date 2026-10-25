package main

import (
	"reflect"
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
