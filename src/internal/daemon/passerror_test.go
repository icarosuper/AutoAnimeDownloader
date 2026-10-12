package daemon

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestPassErrorCode locks the classification of abort causes. It's what decides the sentence the
// user reads in the Status banner: a misclassified cause sends them to check the library folder
// when what went down was AniList.
func TestPassErrorCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"no error", nil, ""},
		{"wrapped anilist", fmt.Errorf("%w: %w", errCauseAnilist, errors.New("anilist returned 403: disabled")), PassErrAnilist},
		// searchAnilist wraps a MAL-only failure in errCauseMAL, and the abort site wraps that in
		// errCauseAnilist: the banner must still blame MAL.
		{"mal inside anilist", fmt.Errorf("%w: %w", errCauseAnilist, fmt.Errorf("%w: boom", errCauseMAL)), PassErrMAL},
		{"incomplete setup", fmt.Errorf("%w: missing folder", errCauseSetup), PassErrSetup},
		{"library without hardlink", fmt.Errorf("%w: %w", errCauseLibrary, errors.New("cross-device link")), PassErrLibrary},
		{"torrent client", fmt.Errorf("%w: not initialized", errCauseTorrent), PassErrTorrent},
		{"app data", fmt.Errorf("%w: %w", errCauseStorage, errors.New("episodes.json corrupted")), PassErrStorage},
		{"unreadable config", fmt.Errorf("%w: %w", errCauseConfig, errors.New("permission denied")), PassErrConfig},
		// A new abort site that forgets to wrap has to land here, not on a wrong cause:
		// "unknown" has its own sentence in the frontend.
		{"unwrapped error", errors.New("anything"), PassErrUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := passErrorCode(tc.err); got != tc.want {
				t.Errorf("passErrorCode = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPassErrorKeepsOriginalMessage: wrapping must not swallow the original text — it still goes
// to the raw `pass_error`, which the screen shows collapsed for whoever opens an issue.
func TestPassErrorKeepsOriginalMessage(t *testing.T) {
	err := fmt.Errorf("%w: %w", errCauseAnilist, errors.New("anilist returned 403: temporarily disabled"))
	if got := err.Error(); got == "" || !errors.Is(err, errCauseAnilist) {
		t.Fatalf("wrapped error lost information: %q", got)
	}
	if want := "temporarily disabled"; !strings.Contains(err.Error(), want) {
		t.Errorf("message = %q, want it to contain %q", err.Error(), want)
	}
}
