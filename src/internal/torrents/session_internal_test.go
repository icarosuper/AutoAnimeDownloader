package torrents

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"testing"
)

func TestParseInfoHashHex(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	magnet := "magnet:?xt=urn:btih:" + hash + "&dn=Some+Anime"
	got, err := parseInfoHash(magnet)
	if err != nil {
		t.Fatalf("parseInfoHash: %v", err)
	}
	if got != hash {
		t.Errorf("got %q, want %q", got, hash)
	}
}

func TestParseInfoHashBase32(t *testing.T) {
	// 20 raw bytes → base32 (32 chars) → expect the hex form back.
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i)
	}
	b32 := base32.StdEncoding.EncodeToString(raw)
	magnet := "magnet:?xt=urn:btih:" + b32
	got, err := parseInfoHash(magnet)
	if err != nil {
		t.Fatalf("parseInfoHash base32: %v", err)
	}
	if got != hex.EncodeToString(raw) {
		t.Errorf("got %q, want %q", got, hex.EncodeToString(raw))
	}
}

func TestParseInfoHashInvalid(t *testing.T) {
	cases := []string{
		"http://not-a-magnet",
		"magnet:?dn=NoXT",
		"magnet:?xt=urn:btih:tooshort",
		"magnet:?xt=urn:sha1:0123456789abcdef0123456789abcdef01234567",
	}
	for _, c := range cases {
		if _, err := parseInfoHash(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestFakeBackendLifecycle(t *testing.T) {
	fb := NewFakeBackend()
	hash := "0123456789abcdef0123456789abcdef01234567"
	magnet := "magnet:?xt=urn:btih:" + hash

	got, err := fb.Add(magnet)
	if err != nil || got != hash {
		t.Fatalf("Add = %q, %v; want %q", got, err, hash)
	}
	// Idempotent add.
	if _, err := fb.Add(magnet); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	if len(fb.List()) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(fb.List()))
	}

	info, ok := fb.Get(hash)
	if !ok || info.Completed {
		t.Fatalf("Get = %+v, %v; want present, not completed", info, ok)
	}

	var completed string
	fb.SetCallbacks(func(h string) { completed = h }, nil)
	fb.CompleteTorrent(hash, "/data/id")
	if completed != hash {
		t.Errorf("onComplete not fired; got %q", completed)
	}
	info, _ = fb.Get(hash)
	if !info.Completed || info.DataDir != "/data/id" {
		t.Errorf("after complete: %+v", info)
	}

	if err := fb.Remove(hash, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := fb.Get(hash); ok {
		t.Errorf("torrent should be gone after Remove")
	}
	// Removing an absent hash is a no-op, mirroring rain's RemoveTorrent (which returns nil
	// for an unknown id) and therefore the real Session.Remove.
	if err := fb.Remove(hash, false); err != nil {
		t.Errorf("Remove of a missing torrent should be a no-op, got %v", err)
	}
}

func TestFakeBackendFail(t *testing.T) {
	fb := NewFakeBackend()
	var gotErr error
	fb.SetCallbacks(nil, func(h string, err error) { gotErr = err })
	fb.FailTorrent("abc", errors.New("boom"))
	if gotErr == nil || gotErr.Error() != "boom" {
		t.Errorf("onFailed not fired correctly: %v", gotErr)
	}
}
