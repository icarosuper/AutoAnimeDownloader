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

// NewSession creates the embedded client. savePath is where torrent files live and keep
// seeding (rain's DataDir); databasePath is the resume database (bbolt), kept outside
// savePath on purpose so resume survives a savePath change.
func NewSession(savePath, databasePath string) (*Session, error) {
	cfg := torrent.DefaultConfig
	cfg.DataDir = savePath
	cfg.Database = databasePath
	cfg.DataDirIncludesTorrentID = true
	// The app does not expose rain's RPC server; disable it to avoid binding a port.
	cfg.RPCEnabled = false

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

func toInfo(t *torrent.Torrent) TorrentInfo {
	return TorrentInfo{
		Hash:      t.InfoHash().String(),
		Name:      t.Name(),
		DataDir:   t.Dir(),
		Completed: t.Stats().Status == torrent.Seeding,
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
