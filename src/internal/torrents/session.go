package torrents

import (
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"AutoAnimeDownloader/src/internal/logger"

	"github.com/cenkalti/rain/v2/torrent"
)

// Session is the embedded BitTorrent client, backed by github.com/cenkalti/rain/v2.
// It is created once at daemon startup and shared across all call sites (replacing the
// per-operation, stateless qBittorrent HTTP client). It implements TorrentBackend.
type Session struct {
	ses *torrent.Session

	mu         sync.RWMutex
	onComplete func(hash string)
	onFailed   func(hash string, err error)
}

// sessionOptions carries the deviations from the production rain config. It exists for the
// tests, which open real sessions and must not depend on the machine being otherwise idle.
type sessionOptions struct {
	// disableDHT turns rain's DHT node off. Creating a session is the only thing that binds a
	// FIXED port — DefaultConfig.DHTPort (7246/udp) — so with DHT on, every test that opens a
	// session fails with "address already in use" whenever the user's daemon (or a second test
	// binary) is already running. Turning it off also keeps the tests off the network, since a
	// DHT node starts talking to the bootstrap routers as soon as it comes up.
	//
	// Production always leaves DHT on: for a Nyaa magnet whose trackers are down, it is the
	// only way left to find peers.
	disableDHT bool
}

// NewSession creates the embedded client. savePath is where torrent files live and keep
// seeding (rain's DataDir); databasePath is the resume database (bbolt), kept outside
// savePath on purpose so resume survives a savePath change.
func NewSession(savePath, databasePath string) (*Session, error) {
	return newSession(savePath, databasePath, sessionOptions{})
}

func newSession(savePath, databasePath string, opts sessionOptions) (*Session, error) {
	cfg := torrent.DefaultConfig
	cfg.DataDir = savePath
	cfg.Database = databasePath
	cfg.DataDirIncludesTorrentID = true
	// The app does not expose rain's RPC server; disable it to avoid binding a port.
	cfg.RPCEnabled = false
	if opts.disableDHT {
		cfg.DHTEnabled = false
	}

	rs, err := torrent.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded torrent session: %w", err)
	}

	return &Session{ses: rs}, nil
}

func (s *Session) Add(magnet string) (string, error) {
	hash, err := parseInfoHash(magnet)
	if err != nil {
		return "", fmt.Errorf("invalid magnet: %w", err)
	}

	// The torrent may already be in the session (e.g. loaded from resume, or re-added by
	// the loop). Check up front instead of matching rain's "duplicate torrent id" error
	// string, which would break silently on any library upgrade that rewords it.
	if s.ses.GetTorrent(hash) != nil {
		logger.Logger.Debug().Str("hash", hash).Msg("Torrent already present, reusing")
		s.armListenerFor(hash)
		return hash, nil
	}

	t, err := s.ses.AddURI(magnet, &torrent.AddTorrentOptions{ID: hash})
	if err != nil {
		return "", fmt.Errorf("failed to add torrent: %w", err)
	}

	s.armListener(t)
	logger.Logger.Info().Str("hash", hash).Str("name", t.Name()).Msg("Added torrent to embedded client")
	return hash, nil
}

func (s *Session) List() []TorrentInfo {
	ts := s.ses.ListTorrents()
	out := make([]TorrentInfo, 0, len(ts))
	for _, t := range ts {
		out = append(out, toInfo(t))
	}
	return out
}

func (s *Session) Get(hash string) (TorrentInfo, bool) {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return TorrentInfo{}, false
	}
	return toInfo(t), true
}

func (s *Session) Remove(hash string, keepData bool) error {
	if err := s.ses.RemoveTorrent(hash, keepData); err != nil {
		return fmt.Errorf("failed to remove torrent %s: %w", hash, err)
	}
	return nil
}

func (s *Session) Pause(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	if err := t.Stop(); err != nil {
		return fmt.Errorf("failed to pause torrent %s: %w", hash, err)
	}
	logger.Logger.Info().Str("hash", hash).Msg("Paused torrent")
	return nil
}

func (s *Session) Resume(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to resume torrent %s: %w", hash, err)
	}
	// Pausing made NotifyStop fire (with a nil error), which ended the one-shot listener
	// goroutine armed by Add/SetCallbacks. Without re-arming, this torrent would finish
	// downloading without ever enqueuing JobOrganize — the episode would never be
	// hardlinked into the library. Re-arming may leave two listeners on a
	// pause/resume/pause/resume torrent; that is safe, because JobOrganize is idempotent.
	s.armListener(t)
	logger.Logger.Info().Str("hash", hash).Msg("Resumed torrent")
	return nil
}

func (s *Session) Announce(hash string) error {
	t := s.ses.GetTorrent(hash)
	if t == nil {
		return fmt.Errorf("torrent %s not found", hash)
	}
	// rain's Announce returns nothing — it queues the announce and returns immediately.
	t.Announce()
	logger.Logger.Info().Str("hash", hash).Msg("Forced torrent re-announce")
	return nil
}

func (s *Session) SetCallbacks(onComplete func(hash string), onFailed func(hash string, err error)) {
	s.mu.Lock()
	s.onComplete = onComplete
	s.onFailed = onFailed
	s.mu.Unlock()

	// Arm listeners for torrents already loaded from resume that are still in progress.
	// Already-completed torrents are handled by startup reconciliation, not by events
	// (their one-shot NotifyComplete already fired before this run).
	for _, t := range s.ses.ListTorrents() {
		if t.Stats().Status != torrent.Seeding {
			s.armListener(t)
		}
	}
}

func (s *Session) Close() error {
	return s.ses.Close()
}

// armListenerFor arms a listener by hash (used when a torrent is already present).
func (s *Session) armListenerFor(hash string) {
	if t := s.ses.GetTorrent(hash); t != nil && t.Stats().Status != torrent.Seeding {
		s.armListener(t)
	}
}

// armListener spawns a one-shot goroutine translating rain's per-torrent events into the
// registered callbacks. NotifyClose is included so the goroutine never leaks when the
// torrent is removed or the session closes.
func (s *Session) armListener(t *torrent.Torrent) {
	hash := t.InfoHash().String()
	go func() {
		select {
		case <-t.NotifyComplete():
			s.mu.RLock()
			cb := s.onComplete
			s.mu.RUnlock()
			if cb != nil {
				cb(hash)
			}
		case err := <-t.NotifyStop():
			if err != nil {
				s.mu.RLock()
				cb := s.onFailed
				s.mu.RUnlock()
				if cb != nil {
					cb(hash, err)
				}
			}
		case <-t.NotifyClose():
		}
	}()
}

// completedFromStats reports whether every piece is on disk. It is deliberately not
// derived from Status: pausing takes a finished torrent out of Seeding, and organize/
// reconcile gate on this field.
func completedFromStats(st torrent.Stats) bool {
	return st.Pieces.Total > 0 && st.Pieces.Have >= st.Pieces.Total
}

func toInfo(t *torrent.Torrent) TorrentInfo {
	// Um Stats() só: é round-trip bloqueante para dentro da goroutine do torrent, não getter.
	st := t.Stats()

	var eta *int64
	if st.ETA != nil {
		secs := int64(st.ETA.Seconds())
		eta = &secs
	}

	return TorrentInfo{
		Hash:             t.InfoHash().String(),
		Name:             t.Name(),
		DataDir:          t.Dir(),
		Completed:        completedFromStats(st),
		Status:           statusSlug(st.Status),
		BytesCompleted:   st.Bytes.Completed,
		BytesTotal:       st.Bytes.Total,
		BytesUploaded:    st.Bytes.Uploaded,
		DownloadSpeed:    st.Speed.Download,
		UploadSpeed:      st.Speed.Upload,
		PeersTotal:       st.Peers.Total,
		PiecesHave:       st.Pieces.Have,
		PiecesTotal:      st.Pieces.Total,
		ETASeconds:       eta,
		SeededForSeconds: int64(st.SeededFor.Seconds()),
	}
}

// parseInfoHash extracts the BitTorrent v1 info hash from a magnet link and returns it as
// lowercase hex (40 chars), matching torrent.InfoHash.String(). The result is used as the
// torrent's ID in rain, so it is kept compatible with rain's own magnet parsing
// (internal/magnet): same hex/base32 accepted forms, same case-sensitive base32 decode.
//
// It is deliberately not a full clone of rain's parser: rain only looks at the first xt
// param, while this scans all of them and prefers `urn:btih:`. That makes hybrid v1/v2
// magnets that list `urn:btmh:` first parse here instead of being rejected outright.
// Only `urn:btih:` (v1) is supported — a magnet with no btih xt is an error.
func parseInfoHash(magnet string) (string, error) {
	u, err := url.Parse(magnet)
	if err != nil {
		return "", err
	}
	if u.Scheme != "magnet" {
		return "", fmt.Errorf("not a magnet link")
	}
	xts := u.Query()["xt"]
	if len(xts) == 0 {
		return "", fmt.Errorf("missing xt param")
	}

	const btihPrefix = "urn:btih:"
	raw := ""
	for _, xt := range xts {
		if strings.HasPrefix(xt, btihPrefix) {
			raw = xt[len(btihPrefix):]
			break
		}
	}
	if raw == "" {
		return "", fmt.Errorf("unsupported xt param(s): %s", strings.Join(xts, ", "))
	}

	var b []byte
	switch len(raw) {
	case 40:
		b, err = hex.DecodeString(raw)
	case 32:
		// Uppercase-only, like rain's base32.StdEncoding.DecodeString — normalizing the
		// case here would accept magnets that rain itself rejects at AddURI time.
		b, err = base32.StdEncoding.DecodeString(raw)
	default:
		return "", fmt.Errorf("info hash must be 32 or 40 characters, got %d", len(raw))
	}
	if err != nil {
		return "", err
	}
	if len(b) != 20 {
		return "", fmt.Errorf("invalid info hash length %d", len(b))
	}
	return hex.EncodeToString(b), nil
}
