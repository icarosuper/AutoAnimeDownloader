package torrents

import (
	"encoding/base32"
	"strings"
	"testing"
)

// Hybrid v1/v2 magnets list both xt params. Nyaa and some trackers emit `urn:btmh:`
// (v2 multihash) first, which used to be rejected because only xts[0] was inspected.
func TestParseInfoHashHybridMagnetBtmhFirst(t *testing.T) {
	const want = "0123456789abcdef0123456789abcdef01234567"
	magnet := "magnet:?xt=urn:btmh:1220caa2b6bd2f8a6e4a3b0e4b6f0f2a1c3d4e5f60718293a4b5c6d7e8f90a1b2c3" +
		"&xt=urn:btih:" + want +
		"&dn=Some+Anime"

	got, err := parseInfoHash(magnet)
	if err != nil {
		t.Fatalf("parseInfoHash on hybrid magnet: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseInfoHashHybridMagnetBtihFirst(t *testing.T) {
	const want = "0123456789abcdef0123456789abcdef01234567"
	magnet := "magnet:?xt=urn:btih:" + want +
		"&xt=urn:btmh:1220caa2b6bd2f8a6e4a3b0e4b6f0f2a1c3d4e5f60718293a4b5c6d7e8f90a1b2c3"

	got, err := parseInfoHash(magnet)
	if err != nil {
		t.Fatalf("parseInfoHash on hybrid magnet: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A base32 info hash also has to be found when it is not the first xt.
func TestParseInfoHashHybridMagnetBase32Btih(t *testing.T) {
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i * 3)
	}
	b32 := base32.StdEncoding.EncodeToString(raw)
	magnet := "magnet:?xt=urn:btmh:1220caa2b6bd2f8a6e4a3b0e4b6f0f2a1c3d4e5f60718293a4b5c6d7e8f90a1b2c3" +
		"&xt=urn:btih:" + b32

	got, err := parseInfoHash(magnet)
	if err != nil {
		t.Fatalf("parseInfoHash base32 in hybrid magnet: %v", err)
	}
	const want = "000306090c0f1215181b1e2124272a2d30333639"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// v2-only magnets carry no v1 info hash, so there is nothing to use as rain's torrent ID.
func TestParseInfoHashBtmhOnlyIsRejected(t *testing.T) {
	magnet := "magnet:?xt=urn:btmh:1220caa2b6bd2f8a6e4a3b0e4b6f0f2a1c3d4e5f60718293a4b5c6d7e8f90a1b2c3"
	_, err := parseInfoHash(magnet)
	if err == nil {
		t.Fatal("expected an error for a v2-only magnet")
	}
	if !strings.Contains(err.Error(), "unsupported xt param") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Base32 decoding is case-sensitive on purpose: it mirrors rain's own parser, so a magnet
// accepted here is one rain will also accept at AddURI time.
func TestParseInfoHashBase32IsCaseSensitive(t *testing.T) {
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i)
	}
	b32 := strings.ToLower(base32.StdEncoding.EncodeToString(raw))
	if _, err := parseInfoHash("magnet:?xt=urn:btih:" + b32); err == nil {
		t.Fatal("expected an error for a lowercase base32 info hash")
	}
}
