package anilist

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// AniList health states. These are the codes the frontend receives and turns into a sentence —
// the same boundary lib/domain/checkIssue.ts documents: the backend sends a CODE, never ready-made
// text, otherwise the daemon would need to know the browser's locale.
const (
	HealthOK          = "ok"
	HealthRateLimited = "rate_limited" // 429: 1-minute timeout, with Retry-After
	HealthOutage      = "outage"       // 403 (API off / IP blocked) or 5xx
	HealthAppBug      = "app_bug"      // 400: the query no longer matches the schema
)

// Health is the last known AniList state. It lives in the package (not in daemon.State) because
// EVERY call goes through sendAnilistRequest: the daemon pass and the /animes poll write here
// without any caller needing to propagate an error. See decisions.md #65.
type Health struct {
	State string `json:"state" example:"ok"`
	// Message is the RAW AniList message. A 403 from a blocked IP brings the reason written for
	// a human to read; it's the only information the frontend cannot reconstruct.
	Message string `json:"message,omitempty"`
	// RetryAt comes from a 429's Retry-After. Zero when it doesn't apply: only the rate limit
	// says how long is left, and that's what lets the banner show a countdown instead of "try
	// again later".
	RetryAt time.Time `json:"retry_at,omitempty"`
	Since   time.Time `json:"since,omitempty"`
	// CacheSavedAt is the date of the on-disk snapshot (persist.go), filled on read and not on
	// writing the state: it isn't a fact about AniList, it's what answers "how old is what I'm
	// looking at?" when AniList is down. Zero when there has never been a snapshot.
	CacheSavedAt time.Time `json:"cache_saved_at,omitempty"`
}

var health atomic.Pointer[Health]

// CurrentHealth returns the current state. Never nil: before the first call the state is "ok",
// because "we haven't asked yet" is no reason to alarm anyone.
func CurrentHealth() Health {
	current := Health{State: HealthOK}
	if h := health.Load(); h != nil {
		current = *h
	}
	current.CacheSavedAt = SnapshotSavedAt()
	return current
}

// setHealth records a degraded state. Preserves the previous state's Since when the failure
// type hasn't changed: the banner shows how long AniList has been down, not how long ago the
// last attempt was.
func setHealth(state, message string, retryAt time.Time) {
	now := time.Now()
	since := now
	if prev := health.Load(); prev != nil && prev.State == state && !prev.Since.IsZero() {
		since = prev.Since
	}
	health.Store(&Health{State: state, Message: message, RetryAt: retryAt, Since: since})
}

// clearHealth marks AniList as healthy. Called on every useful response — the banner goes away
// on the first clean 200, never on a timer: the state either ended or it didn't (see
// decisions.md #66).
func clearHealth() {
	if h := health.Load(); h == nil || h.State != HealthOK {
		health.Store(&Health{State: HealthOK})
	}
}

// graphQLErrors is AniList's error envelope. The official docs are explicit: a 200 can carry
// an error here (invalid query, suppressed field). Without reading this field, that case
// becomes an empty response with no diagnosis — see decisions.md #65.
type graphQLErrors struct {
	Errors []struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"errors"`
}

// firstError extracts the first errors entry from the body. Returns status 0 when there is no
// error at all, which includes the common case of the body not even being a valid GraphQL envelope.
func firstError(body []byte) (status int, message string) {
	var env graphQLErrors
	if err := json.Unmarshal(body, &env); err != nil || len(env.Errors) == 0 {
		return 0, ""
	}
	e := env.Errors[0]
	return e.Status, e.Message
}

// classify turns a code into a health state. The code comes from errors[].status when it exists
// and from HTTP when it doesn't — both paths converge here on purpose, because AniList uses the
// same numbering in both places.
func classify(status int) string {
	switch {
	case status == 429:
		return HealthRateLimited
	case status == 403 || status >= 500:
		return HealthOutage
	case status == 400:
		return HealthAppBug
	default:
		// 401/405/418… aren't documented for this API. Treating them as outage is the safe
		// reading: it tells the user to wait instead of telling them to report a bug that
		// might not be ours.
		return HealthOutage
	}
}

// retryAfter reads the Retry-After header of a 429. AniList sends seconds (never an HTTP date).
//
// Do NOT trust X-RateLimit-Reset: it is advertised in Access-Control-Expose-Headers, but it
// didn't come in any observed 200 response (measured 2026-08-28). If it exists, it's only on
// 429 — unverified, because triggering a 429 blocks the IP for ~1min and the daemon shares
// that IP. X-RateLimit-Limit and X-RateLimit-Remaining, those do come on EVERY response,
// including 400 and 404.
func retryAfter(headerValue string) time.Time {
	secs, err := strconv.Atoi(headerValue)
	if err != nil || secs <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(secs) * time.Second)
}

// Priority declares how much an AniList call can be sacrificed when the request budget is
// running out. There is NO default: the parameter is required on every call, and that's why
// it exists as a parameter instead of a wrapper — a new query can't compile without someone
// deciding its criticality. See decisions.md #72.
type Priority int

const (
	// PriorityCritical is never refused by the gate. It's the daemon pass and every action a
	// user triggered and is waiting on: refusing here is an episode not downloaded or a
	// button that doesn't work.
	PriorityCritical Priority = iota
	// PriorityDisposable is refused when the budget falls below the floor. It's the traffic
	// that repeats on its own — frontend poll, search-as-you-type — and that has somewhere
	// to fall back: expired cache, the next poll, or an error visible to whoever typed.
	PriorityDisposable
)

// ErrBudgetLow is the gate's refusal. It is not an AniList failure: no request even went out,
// and the package's health stays as it was.
var ErrBudgetLow = errors.New("anilist: request budget is low, disposable call refused")

// ErrFromCache accompanies a value that CAME WITH IT: AniList failed and the response came
// from the local cache (possibly from the on-disk snapshot, days old). It isn't "it went
// wrong", it's "it went right with data from another time".
//
// It goes as an error on purpose. Every caller that doesn't know the sentinel sees err != nil
// and behaves as it did before the cache existed — which is the safe behavior. Only whoever
// checks errors.Is opts into using the value, and the choice stays visible at the call site
// instead of hidden in a flag that gets forgotten.
var ErrFromCache = errors.New("anilist: served from the local cache")

const (
	// budgetFloor is how many requests the gate reserves for critical traffic. The measured
	// limit is 30/min per IP (decisions.md #72) and a daemon pass costs a few requests;
	// leaving a third of the bucket aside is enough for it to never bump into the frontend poll.
	budgetFloor = 10
	// budgetValidity is how long a reading lasts, and it's the gate's REQUIRED EXIT: AniList's
	// bucket resets entirely in <= 60s and nothing tells us. Without the validity, a low
	// budget would refuse everyone, nobody would emit a request, no new reading would arrive
	// and the gate would stay stuck forever.
	budgetValidity = 60 * time.Second
)

type budgetReading struct {
	remaining int
	at        time.Time
}

var budget atomic.Pointer[budgetReading]

// recordBudget stores X-RateLimit-Remaining from a response. Called on EVERY response,
// including errors: a 404 and an invalid query consume quota the same as a 200
// (decisions.md #72), so a counter that only looked at success would be optimistic exactly
// when things are already going badly. The header is authoritative because it already sums
// the other consumers of the same IP — none of our counters can see that.
func recordBudget(header http.Header) {
	remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	if err != nil {
		return
	}
	budget.Store(&budgetReading{remaining: remaining, at: time.Now()})
}

// budgetAllows decides whether a call can go out now. Never blocks waiting: the frontend poll
// runs inside an HTTP handler, and hanging the goroutine until the bucket fills would hang
// the screen with it.
func budgetAllows(priority Priority) bool {
	if priority == PriorityCritical {
		return true
	}
	reading := budget.Load()
	if reading == nil || time.Since(reading.at) >= budgetValidity {
		return true
	}
	return reading.remaining >= budgetFloor
}
