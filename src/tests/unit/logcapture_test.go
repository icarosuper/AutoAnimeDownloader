package unit

import (
	"AutoAnimeDownloader/src/internal/logger"
	"bytes"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// syncBuffer is a bytes.Buffer that tolerates concurrent writers.
//
// It exists because AnimeVerification fans out into goroutines that all log through the
// package-level logger: pointing zerolog at a bare bytes.Buffer makes every one of those
// goroutines write to the same unsynchronized buffer. -race reports that as a data race deep
// inside zerolog's writer, several frames away from the test that actually caused it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything logged so far.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs points the package-level logger at a fresh buffer for the duration of the test
// and returns it.
//
// The restore runs from t.Cleanup rather than at the end of the test body: these tests call
// t.Fatal on failure, which skips everything after it, so a hand-written "restore the logger
// last" line leaves the global logger writing into a dead test's buffer — silently swallowing
// the log output every later test in the package asserts on.
func captureLogs(t *testing.T, level zerolog.Level) *syncBuffer {
	t.Helper()

	buf := &syncBuffer{}
	original := logger.Logger
	logger.Logger = zerolog.New(buf).With().Timestamp().Logger().Level(level)
	t.Cleanup(func() { logger.Logger = original })
	return buf
}
