package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockAnimeDetailResponse builds a minimal AniList list-entry JSON body with two airing nodes:
// episode 1 (id 1) and episode 2 (id 2). Used to drive handleAnimeEpisodes without a real
// network call. The route id is the MEDIA id (21), not the entry id (1).
const mockAnimeDetailResponse = `{
	"data": {
		"Page": {
			"mediaList": [
				{
					"id": 1,
					"status": "CURRENT",
					"progress": 0,
					"customLists": {},
					"media": {
						"id": 21,
						"episodes": 24,
						"format": "TV",
						"status": "RELEASING",
						"title": {"english": "Test Anime", "romaji": "Test Anime"},
						"synonyms": [],
						"relations": {"edges": []},
						"coverImage": {"large": "", "medium": ""},
						"airingSchedule": {
							"nodes": [
								{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 111},
								{"id": 2, "episode": 2, "timeUntilAiring": -50, "airingAt": 222}
							]
						}
					}
				}
			]
		}
	}
}`

func mockAnimeDetail() func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockAnimeDetailResponse)),
		}, nil
	})
}

// TestHandleAnimeEpisodes_EpisodeHash covers the episode_hash field added for the inline
// torrent-progress join on the anime detail screen: present (from the saved episode record)
// for a downloaded episode, and omitted from the serialized JSON (via omitempty) for one that
// isn't downloaded. Asserting on the decoded JSON, not the struct, is the point — omitempty
// only has an observable effect on the wire.
func TestHandleAnimeEpisodes_EpisodeHash(t *testing.T) {
	defer mockAnimeDetail()()

	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{
			EpisodeNumber: 1,
			AnimeID:       21,
			EpisodeHash:   "0123456789abcdef0123456789abcdef01234567",
			EpisodeName:   "Test Anime - Episode 1",
			DownloadDate:  time.Now(),
		},
	}}
	server := &Server{FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	w := httptest.NewRecorder()
	handleAnimeEpisodes(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be an object, got %T", response.Data)
	}
	episodes, ok := data["episodes"].([]interface{})
	if !ok || len(episodes) != 2 {
		t.Fatalf("Expected 2 episodes, got %v", data["episodes"])
	}

	ep1, ok := episodes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected episode 1 to be an object")
	}
	if ep1["episode_hash"] != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("episode_hash = %v, want the downloaded episode's hash", ep1["episode_hash"])
	}

	ep2, ok := episodes[1].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected episode 2 to be an object")
	}
	if _, present := ep2["episode_hash"]; present {
		t.Errorf("episode_hash should be omitted (omitempty) for a non-downloaded episode, got %v", ep2["episode_hash"])
	}
}

// Um anime cuja agenda a AniList já clipou (One Piece: a página 1 começa no 1123, e
// nextAiringEpisode é a única pista do que já foi ao ar) tem de aparecer na tela com TODOS os
// episódios — senão não existe onde clicar para baixar os que faltam. Ver decisions.md #52.
func TestHandleAnimeEpisodes_ListsEpisodesOutsideTheAniListWindow(t *testing.T) {
	const body = `{"data":{"Page":{"mediaList":[{
		"id": 1, "status": "CURRENT", "progress": 3, "customLists": {},
		"media": {
			"id": 21, "episodes": null, "format": "TV", "status": "RELEASING",
			"title": {"english": "ONE PIECE", "romaji": "ONE PIECE"},
			"synonyms": [], "relations": {"edges": []}, "coverImage": {"large": "", "medium": ""},
			"airingSchedule": {"nodes": [{"id": 416348, "episode": 10, "timeUntilAiring": -100, "airingAt": 111}]},
			"nextAiringEpisode": {"episode": 12, "timeUntilAiring": 3600, "airingAt": 999}
		}
	}]}}}`

	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})()

	server := &Server{FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/animes/21/episodes", nil)
	req.SetPathValue("id", "21")
	w := httptest.NewRecorder()
	handleAnimeEpisodes(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d (%s)", w.Code, w.Body.String())
	}

	var response struct {
		Data AnimeDetailResponse `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// 1..11 no ar (nextAiringEpisode 12) + o 12 futuro, mesmo com a agenda trazendo só o 10.
	if len(response.Data.Episodes) != 12 {
		t.Fatalf("quero 12 episódios na tela, veio %d", len(response.Data.Episodes))
	}
	for i, ep := range response.Data.Episodes {
		if ep.EpisodeNumber != i+1 {
			t.Fatalf("episódio %d fora de ordem: %+v", i+1, ep)
		}
	}
	if !response.Data.Episodes[0].IsWatched || response.Data.Episodes[3].IsWatched {
		t.Error("is_watched deve seguir o progresso (3) mesmo nos episódios sintetizados")
	}
	if response.Data.Episodes[11].IsAired {
		t.Error("o episódio 12 ainda não foi ao ar")
	}
}
