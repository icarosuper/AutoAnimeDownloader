package anilist

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// respond builds an AniList response with arbitrary body and headers.
func respond(status int, body string, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: h}
}

// TestHealthClassification locks the code -> state mapping. It's what decides which banner the
// user sees: mixing up outage with a bug of ours sends them to report an issue that doesn't
// exist, and mixing up a bug of ours with outage sends them to wait for a fix that never comes.
func TestHealthClassification(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		want    string
		wantMsg string
	}{
		{
			name:    "403 with API off becomes outage and keeps their message",
			status:  403,
			body:    `{"errors":[{"message":"The AniList API has been temporarily disabled due to severe stability issues.","status":403}],"data":null}`,
			want:    HealthOutage,
			wantMsg: "The AniList API has been temporarily disabled due to severe stability issues.",
		},
		{
			name:    "429 becomes rate limit",
			status:  429,
			body:    `{"errors":[{"message":"Too Many Requests.","status":429}],"data":null}`,
			headers: map[string]string{"Retry-After": "30"},
			want:    HealthRateLimited,
			wantMsg: "Too Many Requests.",
		},
		{
			name:    "400 becomes app bug",
			status:  400,
			body:    `{"errors":[{"message":"Cannot query field \"nope\" on type \"MediaList\".","status":400}],"data":null}`,
			want:    HealthAppBug,
			wantMsg: `Cannot query field "nope" on type "MediaList".`,
		},
		{
			name:   "5xx becomes outage",
			status: 502,
			body:   "bad gateway",
			want:   HealthOutage,
		},
		{
			// The case that used to go unnoticed: 200 on HTTP, error in the envelope. See decisions.md #65.
			name:    "200 with errors in the envelope is classified by the inner status",
			status:  200,
			body:    `{"errors":[{"message":"Too Many Requests.","status":429}],"data":null}`,
			want:    HealthRateLimited,
			wantMsg: "Too Many Requests.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
				return respond(tc.status, tc.body, tc.headers), nil
			})
			defer restore()

			if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err == nil {
				t.Fatal("want error, got nil")
			}

			got := CurrentHealth()
			if got.State != tc.want {
				t.Errorf("state = %q, want %q", got.State, tc.want)
			}
			if tc.wantMsg != "" && got.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tc.wantMsg)
			}
			if got.Since.IsZero() {
				t.Error("Since should be set on a degraded state")
			}
		})
	}
}

// TestHealthRetryAtFromHeader: the 429 is the only case where we know how long is left, and
// that's where the banner's countdown comes from. Without it it can only say "try again later".
func TestHealthRetryAtFromHeader(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(429, `{"errors":[{"status":429}]}`, map[string]string{"Retry-After": "30"}), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	got := CurrentHealth().RetryAt
	if got.IsZero() {
		t.Fatal("empty RetryAt: Retry-After was not read")
	}
	if d := time.Until(got); d < 25*time.Second || d > 31*time.Second {
		t.Errorf("RetryAt in %v, want ~30s", d)
	}
}

// TestHealthClearsOnFirstGoodResponse locks the banner rule: it goes away on the first clean
// 200, never on a timer (decisions.md #66).
func TestHealthClearsOnFirstGoodResponse(t *testing.T) {
	fail := true
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		if fail {
			return respond(403, `{"errors":[{"message":"disabled","status":403}]}`, nil), nil
		}
		return respond(200, `{"data":{"Page":{"mediaList":[]}}}`, nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)
	if CurrentHealth().State != HealthOutage {
		t.Fatalf("want outage, got %q", CurrentHealth().State)
	}

	fail = false
	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
		t.Fatalf("good response returned an error: %v", err)
	}
	if got := CurrentHealth(); got.State != HealthOK {
		t.Errorf("state = %q, want %q", got.State, HealthOK)
	}
}

// TestHealthSincePreservedAcrossRepeatedFailures: the banner shows how long AniList has been
// down, not how long ago the last attempt was. On a 30s poll the two are quite different.
func TestHealthSincePreservedAcrossRepeatedFailures(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(403, `{"errors":[{"message":"disabled","status":403}]}`, nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)
	first := CurrentHealth().Since

	time.Sleep(5 * time.Millisecond)
	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if got := CurrentHealth().Since; !got.Equal(first) {
		t.Errorf("Since became %v, want it to stay at %v", got, first)
	}
}

// TestNotFoundIsNotDegradedHealth: a 404 is "this id doesn't exist", not "AniList went down".
// Marking outage here would put a banner on the screen every time the user removed an anime
// from the list.
func TestNotFoundIsNotDegradedHealth(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(404, "", nil), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if got := CurrentHealth().State; got != HealthOK {
		t.Errorf("state = %q, want %q", got, HealthOK)
	}
}
