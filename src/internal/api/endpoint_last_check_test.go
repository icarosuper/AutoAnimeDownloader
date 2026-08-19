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
	t.Run("GET devolve o relatório", func(t *testing.T) {
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
			t.Fatalf("esperava 200, obteve %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("esperava data como objeto")
		}
		if passErr, _ := data["pass_error"].(string); passErr != "" {
			t.Errorf("esperava pass_error vazio, obteve %q", passErr)
		}
		problems, ok := data["problems"].([]interface{})
		if !ok || len(problems) != 1 {
			t.Fatalf("esperava 1 problema, obteve %v", data["problems"])
		}
		first := problems[0].(map[string]interface{})
		if first["code"] != daemon.IssueAllAboveSizeLimit {
			t.Errorf("código errado: %v", first["code"])
		}
		if first["anime_name"] != "Bleach" {
			t.Errorf("anime errado: %v", first["anime_name"])
		}
		limits, ok := data["limits"].([]interface{})
		if !ok || len(limits) != 1 {
			t.Fatalf("esperava 1 limite, obteve %v", data["limits"])
		}
		// omitempty: o limite não carrega episodes.
		if _, present := limits[0].(map[string]interface{})["episodes"]; present {
			t.Error("limite não deve serializar episodes")
		}
	})

	t.Run("GET com relatório vazio", func(t *testing.T) {
		state := daemon.NewState()

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("esperava 200, obteve %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		if !response.Success {
			t.Error("esperava success=true")
		}
	})

	t.Run("pass_error vem do último erro de passe", func(t *testing.T) {
		state := daemon.NewState()
		state.SetLastCheckError(errors.New("anilist unreachable"))

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		data := response.Data.(map[string]interface{})
		if data["pass_error"] != "anilist unreachable" {
			t.Errorf("esperava a mensagem do erro, obteve %v", data["pass_error"])
		}
	})

	t.Run("POST devolve 405", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: daemon.NewState()})(w, httptest.NewRequest(http.MethodPost, "/api/v1/last-check", nil))

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("esperava 405, obteve %d", w.Code)
		}
	})
}
