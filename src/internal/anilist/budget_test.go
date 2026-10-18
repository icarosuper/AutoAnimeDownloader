package anilist

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// budgetTestServer installs a transport that responds 200 with the requested X-RateLimit-Remaining
// and counts how many requests actually went out. It's the only way to verify the gate: a refused
// call produces no effect besides not existing.
func budgetTestServer(t *testing.T, remaining string) *int {
	t.Helper()
	calls := 0
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		calls++
		return respond(200, `{"data":{}}`, map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	t.Cleanup(restore)
	return &calls
}

// TestBudgetGateRefusesDisposableWhenLow: with the bucket at the end, disposable traffic stops
// going out. That's the whole effect of the feature — what's left of the budget stays for the
// daemon pass.
func TestBudgetGateRefusesDisposableWhenLow(t *testing.T) {
	calls := budgetTestServer(t, "3")

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); err != nil {
		t.Fatalf("the first call can't be refused: there is no budget reading yet: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("want 1 request emitted, got %d", *calls)
	}

	_, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable)
	if !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("want ErrBudgetLow, got %v", err)
	}
	if *calls != 1 {
		t.Fatalf("the refused call can't have gone to the network: %d requests", *calls)
	}
}

// TestBudgetGateAlwaysAllowsCritical: the daemon pass is never refused, no matter how low the
// budget is. Refusing here is an episode not downloaded.
func TestBudgetGateAlwaysAllowsCritical(t *testing.T) {
	calls := budgetTestServer(t, "0")

	for i := range 3 {
		if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
			t.Fatalf("critical call %d was refused: %v", i, err)
		}
	}
	if *calls != 3 {
		t.Fatalf("want 3 requests emitted, got %d", *calls)
	}
}

// TestBudgetGateAllowsAgainAfterValidity is the gate's EXIT: AniList's bucket resets entirely
// in <= 60s and nothing tells us. Without the reading's validity, everyone would be refused,
// no request would go out, no new reading would arrive and the gate would never open again.
func TestBudgetGateAllowsAgainAfterValidity(t *testing.T) {
	calls := budgetTestServer(t, "1")

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical); err != nil {
		t.Fatalf("seeding the budget reading: %v", err)
	}
	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("want ErrBudgetLow with the bucket at the end, got %v", err)
	}

	// Age the reading instead of waiting 60s of clock.
	budget.Store(&budgetReading{remaining: 1, at: time.Now().Add(-budgetValidity - time.Second)})

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); err != nil {
		t.Fatalf("an expired reading has to release the gate, got %v", err)
	}
	if *calls != 2 {
		t.Fatalf("want 2 requests emitted, got %d", *calls)
	}
}

// TestBudgetRecordedOnErrorResponse: a 404 and an invalid query consume quota the same as a 200
// (decisions.md #72). A gate that only read a good response would be optimistic exactly when
// things are already going badly.
func TestBudgetRecordedOnErrorResponse(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(404, "", map[string]string{"X-RateLimit-Remaining": "2"}), nil
	})
	defer restore()

	_, _ = sendAnilistRequest[AniListResponse]("query{}", nil, PriorityCritical)

	if _, err := sendAnilistRequest[AniListResponse]("query{}", nil, PriorityDisposable); !errors.Is(err, ErrBudgetLow) {
		t.Fatalf("the 404 header had to feed the gate, got %v", err)
	}
}

// TestCustomListsServesStaleCacheWhenRefused is the pair of the test below, and it exists
// because the asymmetry between the two broke the blacklist: the list comes from the expired
// cache and responds with success, but the refused customLists came back nil and the GET
// /animes merge concluded that no anime is in an excluded list.
func TestCustomListsServesStaleCacheWhenRefused(t *testing.T) {
	remaining := "29"
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"mediaList":[{"id":7,"customLists":{"Blacklist":true}}]}}}`,
			map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	defer restore()

	statuses := []string{"CURRENT"}
	if m := GetCustomListsMap("user", statuses, PriorityDisposable); len(m) != 1 {
		t.Fatalf("the first fetch should populate the cache, got %+v", m)
	}

	// Expire the cache and sink the budget: the next call is refused by the gate.
	customListsCache.set("user\x00CURRENT", map[int]CustomLists{7: {"Blacklist": true}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	m := GetCustomListsMap("user", statuses, PriorityDisposable)
	if !m[7]["Blacklist"] {
		t.Fatalf("want the expired entry with the blacklist, got %+v", m)
	}
}

// TestFrontendListServesStaleCacheWhenRefused locks the fallback from the table in
// decisions.md #72: a refused frontend poll serves the expired reading instead of taking
// the screen down.
func TestFrontendListServesStaleCacheWhenRefused(t *testing.T) {
	remaining := "29"
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, `{"data":{"Page":{"mediaList":[{"id":7}]}}}`,
			map[string]string{"X-RateLimit-Remaining": remaining}), nil
	})
	defer restore()

	statuses := []string{"CURRENT"}
	if _, err := GetFrontendAnimeList("user", statuses); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Expire the cache and sink the budget: the next call is refused by the gate.
	frontendListCache.set("user\x00CURRENT", []MediaList{{Id: 7}}, -time.Second)
	budget.Store(&budgetReading{remaining: 1, at: time.Now()})

	resp, err := GetFrontendAnimeList("user", statuses)
	if err != nil {
		t.Fatalf("want the expired cache, got error: %v", err)
	}
	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Id != 7 {
		t.Fatalf("want the expired entry, got %+v", resp.Data.Page.MediaList)
	}
}
