// Package updates asks GitHub whether a newer release of the app exists. It never blocks anything
// and never fails loudly: an unreachable GitHub is a Debug line and another try tomorrow.
package updates

import (
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAPIURL = "https://api.github.com"
	releasePath   = "/repos/icarosuper/AutoAnimeDownloader/releases/latest"
)

// The first check waits so it doesn't compete with the first verification pass; then once a day.
// The unauthenticated GitHub limit is 60 requests an hour per IP, so a day is nothing. Vars only so
// the tests can shorten them.
var (
	firstCheckDelay = 2 * time.Minute
	checkEvery      = 24 * time.Hour
)

var (
	mu        sync.RWMutex
	latest    string
	latestURL string
)

// Latest is the newer version found by the last check and its release page, both "" when there is
// none: no newer version, the check is off, it has not run yet, or the build is "dev".
func Latest() (version, url string) {
	mu.RLock()
	defer mu.RUnlock()
	return latest, latestURL
}

func setLatest(version, url string) {
	mu.Lock()
	defer mu.Unlock()
	latest, latestURL = version, url
}

// Start runs the checker until ctx ends. A "dev" build never starts it. enabled is asked before
// every check, so turning check_for_updates off stops the requests without a restart (and clears
// what Latest reports). onNewer gets every newer version found — deduping the announcement is the
// caller's.
func Start(ctx context.Context, current string, enabled func() bool, onNewer func(version, url string)) {
	if current == "dev" || current == "" {
		return
	}
	go func() {
		wait := firstCheckDelay
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = checkEvery
			if !enabled() {
				setLatest("", "")
				continue
			}
			version, url, err := fetchLatest(ctx, apiURL())
			if err != nil {
				logger.Logger.Debug().Err(err).Msg("Update check failed; trying again tomorrow")
				continue
			}
			if !Newer(version, current) {
				setLatest("", "")
				continue
			}
			setLatest(version, url)
			onNewer(version, url)
		}
	}()
}

func apiURL() string {
	if u := os.Getenv("GITHUB_API_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultAPIURL
}

// fetchLatest returns the latest release's version (no "v") and page. releases/latest already
// leaves out drafts and prereleases.
func fetchLatest(ctx context.Context, base string) (version, url string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+releasePath, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github answered %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", fmt.Errorf("unreadable release: %w", err)
	}
	version = strings.TrimPrefix(body.TagName, "v")
	if _, _, ok := parse(version); !ok {
		return "", "", fmt.Errorf("unparsable tag %q", body.TagName)
	}
	return version, body.HTMLURL, nil
}

// Newer reports whether a is a later version than b ("2.11.0" vs "2.10.3"; a leading "v" is fine).
// A prerelease of the same numbers is older than the release: 2.10.0 is newer than 2.10.0-rc1.
// An unparsable side is never newer. No semver dependency: the tags are always X.Y.Z.
func Newer(a, b string) bool {
	an, apre, ok := parse(a)
	if !ok {
		return false
	}
	bn, bpre, ok := parse(b)
	if !ok {
		return false
	}
	for i := range an {
		if an[i] != bn[i] {
			return an[i] > bn[i]
		}
	}
	return apre == "" && bpre != ""
}

func parse(v string) (nums [3]int, pre string, ok bool) {
	v = strings.TrimPrefix(v, "v")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}
