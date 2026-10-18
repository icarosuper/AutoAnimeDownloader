package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errBadEBML marks an invalid variable-length integer byte. Never leaks out of the package: every
// error path here becomes "" in VideoCodec.
var errBadEBML = errors.New("invalid EBML variable-length integer")

// Video codec read from the file itself, not from its name.
//
// Why this exists: `nyaa.ExtractCodec` reads the codec from the release NAME, and a lot of uploads
// simply don't write it — Erai-raws doesn't, many BD rips don't. The only place the codec is
// always present is the file's own header, declared by whoever encoded it.
//
// Matroska (`.mkv`) is an EBML document (RFC 8794): a tree of `ID + size + content` elements
// with variable-width sizes — binary XML. CodecID lives on a fixed, SHORT path, before the
// Clusters (the video itself):
//
//	Segment → Tracks → TrackEntry → { TrackType (1 = video), CodecID }
//
// That's why the read costs a few KB and a few seeks, not the whole file.
//
// Matroska only: MP4 has an analogous structure (boxes) but would need a second parser, and
// anime is almost all mkv — anything else falls through to `""`, which the UI shows as nothing.

// EBML/Matroska element IDs, written WITH the size-marker bits, which is how the Matroska
// spec tables them.
const (
	idEBMLHeader = 0x1A45DFA3
	idSegment    = 0x18538067
	idTracks     = 0x1654AE6B
	idCluster    = 0x1F43B675
	idTrackEntry = 0xAE
	idTrackType  = 0x83
	idCodecID    = 0x86

	trackTypeVideo = 1
	// sizeUnknown is EBML's "unknown" size (all value bits set). A Segment written live
	// uses this; descending into it is valid, skipping it is not.
	sizeUnknown = ^uint64(0)
)

// mkvCodecLabels maps Matroska CodecIDs to the SAME vocabulary `nyaa.ExtractCodec` uses
// elsewhere ("HEVC", "H.264", "AV1"), otherwise the priorities screen and this panel
// would talk about codecs with different names. Anything not in the map comes out as the
// CodecID without the `V_` prefix — ugly, but honest: better than guessing a pretty wrong
// label.
var mkvCodecLabels = map[string]string{
	"V_MPEGH/ISO/HEVC": "HEVC",
	"V_MPEG4/ISO/AVC":  "H.264",
	"V_AV1":            "AV1",
	"V_VP9":            "VP9",
	"V_VP8":            "VP8",
	"V_MPEG4/ISO/ASP":  "MPEG-4",
	"V_MPEG2":          "MPEG-2",
}

// VideoCodec returns the codec of the first video track of a `.mkv`, in `nyaa.ExtractCodec`
// vocabulary. Returns "" — never an error — for anything that cannot be answered: file that
// won't open, isn't Matroska, doesn't have the header on disk yet, or EBML that doesn't match.
// Display data: not knowing is a normal state, not a failure.
func VideoCodec(path string) string {
	if !strings.EqualFold(filepath.Ext(path), ".mkv") {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// The EBML header only says "this is EBML, version X, DocType matroska" — it is a
	// signature and is skipped whole; what matters is in the Segment.
	id, size, err := readElementHeader(f)
	if err != nil || id != idEBMLHeader || size == sizeUnknown {
		return ""
	}
	if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
		return ""
	}

	if _, ok := seekToChild(f, idSegment); !ok {
		return ""
	}
	tracksSize, ok := seekToChild(f, idTracks)
	if !ok {
		return ""
	}
	// LimitReader by the declared Tracks size: without it the scan would walk past the end of
	// Tracks and start reading Cluster as if it were a TrackEntry.
	return videoCodecFromTracks(io.LimitReader(f, int64(tracksSize)))
}

// seekToChild skips sibling elements until it finds `want`, leaving the file positioned at
// its CONTENT. Stops at Cluster: the video has started and Tracks does not come after that.
func seekToChild(f *os.File, want uint32) (uint64, bool) {
	for {
		id, size, err := readElementHeader(f)
		if err != nil {
			return 0, false
		}
		if id == want {
			return size, size != sizeUnknown
		}
		if id == idCluster || size == sizeUnknown {
			// Cluster is the video itself: Tracks comes before, so going past here is giving up.
			// Unknown size on something that isn't the target cannot be skipped without parsing
			// the whole content, and it isn't worth it — "I don't know" is also an answer.
			return 0, false
		}
		if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
			return 0, false
		}
	}
}

// videoCodecFromTracks walks TrackEntry elements and returns the CodecID of the first one
// that declares itself video. An anime mkv has video, audio and several subtitles — taking
// the first TrackEntry without looking at TrackType would return the audio codec on a file
// with swapped order.
func videoCodecFromTracks(r io.Reader) string {
	for {
		id, size, err := readElementHeader(r)
		if err != nil || size == sizeUnknown {
			return ""
		}
		if id != idTrackEntry {
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return ""
			}
			continue
		}

		entry := io.LimitReader(r, int64(size))
		codec, isVideo := parseTrackEntry(entry)
		// Drain the rest of the entry so the next sibling starts in the right place, even when
		// parseTrackEntry stopped early.
		if _, err := io.Copy(io.Discard, entry); err != nil {
			return ""
		}
		if isVideo && codec != "" {
			if label, ok := mkvCodecLabels[codec]; ok {
				return label
			}
			return strings.TrimPrefix(codec, "V_")
		}
	}
}

// parseTrackEntry reads TrackType and CodecID from inside ONE TrackEntry.
func parseTrackEntry(r io.Reader) (codec string, isVideo bool) {
	for {
		id, size, err := readElementHeader(r)
		if err != nil || size == sizeUnknown {
			return codec, isVideo
		}
		switch id {
		case idTrackType:
			n, err := readUint(r, size)
			if err != nil {
				return codec, isVideo
			}
			isVideo = n == trackTypeVideo
		case idCodecID:
			b := make([]byte, size)
			if _, err := io.ReadFull(r, b); err != nil {
				return codec, isVideo
			}
			// CodecID is an ASCII string, but the spec allows NUL padding on the right.
			codec = strings.TrimRight(string(b), "\x00")
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return codec, isVideo
			}
		}
	}
}

// readElementHeader reads the (ID, size) pair that opens every EBML element.
func readElementHeader(r io.Reader) (uint32, uint64, error) {
	id, err := readID(r)
	if err != nil {
		return 0, 0, err
	}
	size, err := readSize(r)
	if err != nil {
		return 0, 0, err
	}
	return id, size, nil
}

// vintLen returns the byte length of an EBML variable-width integer, read from the
// position of the first 1-bit of the first byte. 0 means an invalid byte (0x00).
func vintLen(b byte) int {
	for i := 0; i < 8; i++ {
		if b&(0x80>>i) != 0 {
			return i + 1
		}
	}
	return 0
}

// readID reads an element ID (1 to 4 bytes), returned WITH the marker bits — that's how
// the Matroska spec tables the IDs, and that's what the constants above expect.
func readID(r io.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return 0, err
	}
	n := vintLen(b[0])
	if n == 0 || n > 4 {
		return 0, errBadEBML
	}
	if n > 1 {
		if _, err := io.ReadFull(r, b[1:n]); err != nil {
			return 0, err
		}
	}
	var id uint32
	for i := 0; i < n; i++ {
		id = id<<8 | uint32(b[i])
	}
	return id, nil
}

// readSize reads an element size (1 to 8 bytes), already WITHOUT the marker bit.
func readSize(r io.Reader) (uint64, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return 0, err
	}
	n := vintLen(b[0])
	if n == 0 {
		return 0, errBadEBML
	}
	if n > 1 {
		if _, err := io.ReadFull(r, b[1:n]); err != nil {
			return 0, err
		}
	}
	value := uint64(b[0] &^ (0x80 >> (n - 1)))
	for i := 1; i < n; i++ {
		value = value<<8 | uint64(b[i])
	}
	// All value bits set = unknown size.
	if value == (uint64(1)<<(7*n))-1 {
		return sizeUnknown, nil
	}
	return value, nil
}

// readUint reads an EBML integer of `size` bytes (big-endian, unsigned).
func readUint(r io.Reader, size uint64) (uint64, error) {
	if size > 8 {
		return 0, errBadEBML
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, err
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, nil
}
