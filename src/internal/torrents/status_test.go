package torrents

import (
	"testing"

	"github.com/cenkalti/rain/v2/torrent"
)

func TestStatusSlug(t *testing.T) {
	cases := []struct {
		status torrent.Status
		want   string
	}{
		{torrent.Stopped, "stopped"},
		{torrent.DownloadingMetadata, "downloading_metadata"},
		{torrent.Allocating, "allocating"},
		{torrent.Verifying, "verifying"},
		{torrent.Downloading, "downloading"},
		{torrent.Seeding, "seeding"},
		{torrent.Stopping, "stopping"},
	}

	for _, c := range cases {
		if got := statusSlug(c.status); got != c.want {
			t.Errorf("statusSlug(%v) = %q, want %q", c.status, got, c.want)
		}
	}
}

// A rain pode ganhar estados novos numa atualização. Um status desconhecido tem que virar
// "unknown", nunca string vazia — a UI usa o slug como chave de tradução.
func TestStatusSlugUnknown(t *testing.T) {
	if got := statusSlug(torrent.Status(99)); got != "unknown" {
		t.Errorf("statusSlug(99) = %q, want %q", got, "unknown")
	}
}

// Os slugs são contrato de API: precisam ser únicos, minúsculos e sem espaço, ao contrário
// do Status.String() da rain, que devolve display text ("Downloading Metadata").
func TestStatusSlugsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	all := []torrent.Status{
		torrent.Stopped, torrent.DownloadingMetadata, torrent.Allocating,
		torrent.Verifying, torrent.Downloading, torrent.Seeding, torrent.Stopping,
	}
	for _, s := range all {
		slug := statusSlug(s)
		if seen[slug] {
			t.Errorf("duplicate slug %q for status %v", slug, s)
		}
		seen[slug] = true
	}
}

// TestCompletionByPieceCount validates that completedFromStats(st) returns true when
// all pieces are downloaded, even if the torrent status is Stopped (paused). This ensures
// organize/reconcile logic gates on true completion, not Status enum. The function is
// called by toInfo (session.go), which requires a live rain session to fully validate;
// this test exercises the predicate with constructed torrent.Stats values.
func TestCompletionByPieceCount(t *testing.T) {
	cases := []struct {
		name string
		st   torrent.Stats
		want bool
	}{
		{
			name: "all pieces, torrent complete",
			st: torrent.Stats{
				Pieces: struct {
					Checked   uint32
					Have      uint32
					Missing   uint32
					Available uint32
					Total     uint32
				}{Total: 100, Have: 100},
			},
			want: true,
		},
		{
			name: "all pieces, single piece",
			st: torrent.Stats{
				Pieces: struct {
					Checked   uint32
					Have      uint32
					Missing   uint32
					Available uint32
					Total     uint32
				}{Total: 1, Have: 1},
			},
			want: true,
		},
		{
			name: "incomplete, downloading",
			st: torrent.Stats{
				Pieces: struct {
					Checked   uint32
					Have      uint32
					Missing   uint32
					Available uint32
					Total     uint32
				}{Total: 100, Have: 50},
			},
			want: false,
		},
		{
			name: "incomplete, just started",
			st: torrent.Stats{
				Pieces: struct {
					Checked   uint32
					Have      uint32
					Missing   uint32
					Available uint32
					Total     uint32
				}{Total: 100, Have: 1},
			},
			want: false,
		},
		{
			name: "no pieces, no metadata",
			st: torrent.Stats{
				Pieces: struct {
					Checked   uint32
					Have      uint32
					Missing   uint32
					Available uint32
					Total     uint32
				}{Total: 0, Have: 0},
			},
			want: false,
		},
	}

	for _, c := range cases {
		got := completedFromStats(c.st)
		if got != c.want {
			t.Errorf("%s: completedFromStats = %v, want %v (total=%d, have=%d)",
				c.name, got, c.want, c.st.Pieces.Total, c.st.Pieces.Have)
		}
	}
}
