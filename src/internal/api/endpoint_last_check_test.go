package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleLastCheck(t *testing.T) {
	t.Run("GET returns the report", func(t *testing.T) {
		state := daemon.NewState()
		state.SetLastCheckReport(daemon.CheckReport{
			FinishedAt: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			Problems: []daemon.Issue{{
				AnimeID:    269,
				AnimeName:  "Bleach",
				Episodes:   []int{12, 15},
				Code:       daemon.IssueAllAboveSizeLimit,
				Candidates: 8,
				LimitGB:    3,
			}},
			Limits: []daemon.Issue{{
				AnimeID:      21,
				AnimeName:    "One Piece",
				Code:         daemon.IssueMaxEpisodesPerAnime,
				Downloaded:   12,
				Pending:      35,
				BatchSkipped: daemon.BatchSkippedNoResult,
			}},
		})

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("invalid response: %v", err)
		}
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("expected data as an object")
		}
		if passErr, _ := data["pass_error"].(string); passErr != "" {
			t.Errorf("expected empty pass_error, got %q", passErr)
		}
		problems, ok := data["problems"].([]interface{})
		if !ok || len(problems) != 1 {
			t.Fatalf("expected 1 problem, got %v", data["problems"])
		}
		first := problems[0].(map[string]interface{})
		if first["code"] != daemon.IssueAllAboveSizeLimit {
			t.Errorf("wrong code: %v", first["code"])
		}
		if first["anime_name"] != "Bleach" {
			t.Errorf("wrong anime: %v", first["anime_name"])
		}
		limits, ok := data["limits"].([]interface{})
		if !ok || len(limits) != 1 {
			t.Fatalf("expected 1 limit, got %v", data["limits"])
		}
		// omitempty: the limit carries no episodes.
		if _, present := limits[0].(map[string]interface{})["episodes"]; present {
			t.Error("a limit must not serialize episodes")
		}
	})

	// The Status screen reads .length of both lists: a null one froze it on "Loading status..." after
	// any pass that applied no limit.
	t.Run("GET with an empty report returns empty lists, not null", func(t *testing.T) {
		state := daemon.NewState()

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("invalid response: %v", err)
		}
		if !response.Success {
			t.Error("expected success=true")
		}
		data := response.Data.(map[string]interface{})
		for _, field := range []string{"problems", "limits"} {
			if list, ok := data[field].([]interface{}); !ok || len(list) != 0 {
				t.Errorf("expected %s as an empty list, got %#v", field, data[field])
			}
		}
	})

	t.Run("pass_error comes from the last pass error", func(t *testing.T) {
		state := daemon.NewState()
		state.SetLastCheckError(errors.New("anilist unreachable"))

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("invalid response: %v", err)
		}
		data := response.Data.(map[string]interface{})
		if data["pass_error"] != "anilist unreachable" {
			t.Errorf("expected the error message, got %v", data["pass_error"])
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: daemon.NewState()})(w, httptest.NewRequest(http.MethodPost, "/api/v1/last-check", nil))

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", w.Code)
		}
	})
}
