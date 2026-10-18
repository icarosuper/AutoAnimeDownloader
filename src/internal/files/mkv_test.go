package files

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --- build a minimal .mkv so the test does not depend on a binary file in the repo ---

// ebmlID writes an ID already with the marker bits (the constants in mkv.go use that form).
func ebmlID(id uint32) []byte {
	switch {
	case id <= 0xFF:
		return []byte{byte(id)}
	case id <= 0xFFFF:
		return []byte{byte(id >> 8), byte(id)}
	case id <= 0xFFFFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	}
}

// ebmlSize writes a size in 4 bytes (marker 0b0001xxxx). Fixed width on purpose: what this
// test exercises is navigation, not the most compact packing possible.
func ebmlSize(n uint64) []byte {
	return []byte{0x10 | byte(n>>24), byte(n >> 16), byte(n >> 8), byte(n)}
}

func elem(id uint32, payload []byte) []byte {
	out := append([]byte{}, ebmlID(id)...)
	out = append(out, ebmlSize(uint64(len(payload)))...)
	return append(out, payload...)
}

// buildMKV assembles a Matroska with an EBML header, a fat element before Tracks (so the
// skip-by-size is actually exercised) and the tracks in the requested order.
func buildMKV(tracks ...[]byte) []byte {
	var segment []byte
	// A 100 KB "SeekHead" the parser has to skip by declared size, without reading it.
	segment = append(segment, elem(0x114D9B74, bytes.Repeat([]byte{0xAA}, 100*1024))...)
	var trackList []byte
	for _, t := range tracks {
		trackList = append(trackList, t...)
	}
	segment = append(segment, elem(idTracks, trackList)...)
	// Cluster after Tracks, as in a real file.
	segment = append(segment, elem(idCluster, bytes.Repeat([]byte{0xBB}, 4096))...)

	var out []byte
	out = append(out, elem(idEBMLHeader, []byte{0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a'})...)
	return append(out, elem(idSegment, segment)...)
}

func track(trackType byte, codecID string) []byte {
	var body []byte
	body = append(body, elem(idTrackType, []byte{trackType})...)
	body = append(body, elem(idCodecID, []byte(codecID))...)
	// An unknown element in the middle of the entry: the parser has to skip it, not choke.
	body = append(body, elem(0xD7, []byte{0x01})...)
	return elem(idTrackEntry, body)
}

func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestVideoCodecReadsTheCodecIDFromTheHeader(t *testing.T) {
	cases := []struct {
		codecID string
		want    string
	}{
		{"V_MPEGH/ISO/HEVC", "HEVC"},
		{"V_MPEG4/ISO/AVC", "H.264"},
		{"V_AV1", "AV1"},
		// Off the map: the CodecID comes out without the prefix, not a guessed pretty label.
		{"V_QUICKTIME", "QUICKTIME"},
	}
	for _, tc := range cases {
		path := writeTemp(t, "ep.mkv", buildMKV(track(1, tc.codecID)))
		if got := VideoCodec(path); got != tc.want {
			t.Errorf("VideoCodec(%s) = %q, want %q", tc.codecID, got, tc.want)
		}
	}
}

// An anime mkv has video, audio and several subtitles. Taking the first TrackEntry without
// looking at TrackType would return FLAC as if it were the video codec.
func TestVideoCodecSkipsNonVideoTracks(t *testing.T) {
	path := writeTemp(t, "ep.mkv", buildMKV(
		track(2, "A_FLAC"),
		track(17, "S_TEXT/ASS"),
		track(1, "V_MPEGH/ISO/HEVC"),
	))
	if got := VideoCodec(path); got != "HEVC" {
		t.Errorf("VideoCodec = %q, want HEVC", got)
	}
}

// Every "cannot answer" path returns "" — never an error, never a panic. Display data:
// not knowing is a normal state.
func TestVideoCodecUnknownCasesReturnEmpty(t *testing.T) {
	mkv := buildMKV(track(1, "V_AV1"))

	cases := []struct {
		name string
		path string
	}{
		{"extension other than mkv", writeTemp(t, "ep.mp4", mkv)},
		{"missing file", filepath.Join(t.TempDir(), "missing.mkv")},
		{"empty file", writeTemp(t, "empty.mkv", nil)},
		{"not EBML", writeTemp(t, "junk.mkv", bytes.Repeat([]byte{0x00}, 512))},
		// Header not downloaded yet: rain does not download sequentially, so an in-flight
		// file can be a hole of zeros with the signature missing.
		{"truncated before Tracks", writeTemp(t, "partial.mkv", mkv[:64])},
		{"audio only", writeTemp(t, "audio.mkv", buildMKV(track(2, "A_FLAC")))},
	}
	for _, tc := range cases {
		if got := VideoCodec(tc.path); got != "" {
			t.Errorf("%s: VideoCodec = %q, want \"\"", tc.name, got)
		}
	}
}
