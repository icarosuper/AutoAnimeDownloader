package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// readBody reads and returns the request body as a string without consuming it.
func readBody(req *http.Request) string {
	b, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(strings.NewReader(string(b)))
	return string(b)
}

func mockAniListEmpty() func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		body := `{"data":{"Page":{"mediaList":[]}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
}

func TestHandleAnimes(t *testing.T) {
	state := daemon.NewState()
	mockFM := &mockFileManager{
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, EpisodeName: "Test Anime - Episode 1", DownloadDate: time.Now()},
			{EpisodeNumber: 2, EpisodeName: "Test Anime - Episode 2", DownloadDate: time.Now()},
			{EpisodeNumber: 3, EpisodeName: "Another Anime - Episode 1", DownloadDate: time.Now()},
		},
	}
	server := &Server{
		State:       state,
		FileManager: mockFM,
	}

	handler := handleAnimes(server)

	t.Run("GET returns animes list", func(t *testing.T) {
		defer mockAniListEmpty()()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Error("Expected success=true")
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 2 {
			t.Errorf("Expected 2 animes, got %d", len(animes))
		}
	})

	t.Run("Non-GET method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})

	t.Run("Empty episodes list returns empty array", func(t *testing.T) {
		defer mockAniListEmpty()()
		mockFMEmpty := &mockFileManager{
			episodes: []files.EpisodeStruct{},
		}
		serverEmpty := &Server{
			State:       state,
			FileManager: mockFMEmpty,
		}
		handlerEmpty := handleAnimes(serverEmpty)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerEmpty(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 0 {
			t.Errorf("Expected 0 animes, got %d", len(animes))
		}
	})

	t.Run("CURRENT anime with no downloaded episodes still appears in list", func(t *testing.T) {
		english := "Vivy: Fluorite Eye's Song"
		totalEps := 13
		anilistBody := `{"data":{"Page":{"mediaList":[{"id":900042,"status":"CURRENT","progress":0,"customLists":{},"media":{"id":42,"format":"TV","status":"RELEASING","episodes":13,"title":{"english":"Vivy: Fluorite Eye's Song","romaji":"Vivy: Fluorite Eye's Song"},"airingSchedule":{"nodes":[]}}}]}}}`
		_ = english
		_ = totalEps
		defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(anilistBody)),
			}, nil
		})()

		mockFMNoEps := &mockFileManager{episodes: []files.EpisodeStruct{}}
		serverNoEps := &Server{State: state, FileManager: mockFMNoEps}
		handlerNoEps := handleAnimes(serverNoEps)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerNoEps(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 1 {
			t.Errorf("Expected 1 anime from AniList merge, got %d", len(animes))
		}

		animeData := animes[0].(map[string]interface{})
		if animeData["name"] != "Vivy: Fluorite Eye's Song" {
			t.Errorf("Expected anime name 'Vivy: Fluorite Eye's Song', got %v", animeData["name"])
		}
		if animeData["episodes_downloaded"].(float64) != 0 {
			t.Errorf("Expected episodes_downloaded=0, got %v", animeData["episodes_downloaded"])
		}
		if animeData["anime_id"].(float64) != 42 {
			t.Errorf("Expected anime_id=42 (media id), got %v", animeData["anime_id"])
		}
	})

	t.Run("Blacklisted anime has is_blacklisted=true in response", func(t *testing.T) {
		anilistBody := `{"data":{"Page":{"mediaList":[{"id":99,"status":"CURRENT","progress":0,"customLists":{"Blacklist":true},"media":{"id":99,"format":"TV","status":"RELEASING","episodes":12,"title":{"english":"Blacklisted Anime","romaji":"Blacklisted Anime"},"airingSchedule":{"nodes":[]}}}]}}}`
		defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(anilistBody)),
			}, nil
		})()

		mockFMBlacklist := &mockFileManager{
			episodes: []files.EpisodeStruct{},
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				ExcludedLists:         []string{"Blacklist"},
				DownloadStatuses:      []string{"CURRENT", "REPEATING"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverBlacklist := &Server{State: state, FileManager: mockFMBlacklist}
		handlerBlacklist := handleAnimes(serverBlacklist)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerBlacklist(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("Expected data to be an array")
		}

		if len(animes) != 1 {
			t.Fatalf("Expected 1 anime, got %d", len(animes))
		}

		animeData := animes[0].(map[string]interface{})
		isBlacklisted, exists := animeData["is_blacklisted"]
		if !exists {
			t.Error("Expected is_blacklisted field in response")
		} else if isBlacklisted != true {
			t.Errorf("Expected is_blacklisted=true, got %v", isBlacklisted)
		}
	})

	t.Run("Blacklisted anime detected when complex query returns null customLists (Anilist rate-limit overlay)", func(t *testing.T) {
		// Simulates the Anilist query-complexity bug: GetAllCurrentAnime returns customLists:null
		// for rate-limited/complex queries, but GetCustomListsMap (lightweight) returns valid data.
		// The overlay in fetchAniListEntries must restore customLists so IsBlacklisted is set.
		lightweightBody := `{"data":{"Page":{"mediaList":[{"id":77,"customLists":{"Blacklist":true}}]}}}`
		complexBody := `{"data":{"Page":{"mediaList":[{"id":77,"status":"CURRENT","progress":0,"customLists":null,"media":{"id":77,"format":"TV","status":"RELEASING","episodes":12,"title":{"english":"Rate Limited Anime","romaji":"Rate Limited Anime"},"coverImage":{"large":"","medium":""},"synonyms":[],"relations":{"edges":[]},"airingSchedule":{"nodes":[]}}}]}}}`

		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			body := readBody(req)
			respBody := lightweightBody
			if strings.Contains(body, "coverImage") {
				respBody = complexBody
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
			}, nil
		})()

		mockFMRL := &mockFileManager{
			episodes: []files.EpisodeStruct{},
			configs: &files.Config{
				AnilistUsernames:      []string{"rateLimitUser"},
				ExcludedLists:         []string{"Blacklist"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverRL := &Server{State: state, FileManager: mockFMRL}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverRL)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes, ok := response.Data.([]interface{})
		if !ok || len(animes) != 1 {
			t.Fatalf("Expected 1 anime, got %d", len(animes))
		}

		animeData := animes[0].(map[string]interface{})
		isBlacklisted, exists := animeData["is_blacklisted"]
		if !exists {
			t.Error("Expected is_blacklisted=true: overlay must restore customLists when complex query returns null")
		} else if isBlacklisted != true {
			t.Errorf("Expected is_blacklisted=true, got %v", isBlacklisted)
		}
	})

	t.Run("Non-blacklisted anime has no is_blacklisted field in response", func(t *testing.T) {
		anilistBody := `{"data":{"Page":{"mediaList":[{"id":100,"status":"CURRENT","progress":0,"customLists":{"Blacklist":false},"media":{"format":"TV","status":"RELEASING","episodes":12,"title":{"english":"Normal Anime","romaji":"Normal Anime"},"airingSchedule":{"nodes":[]}}}]}}}`
		defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(anilistBody)),
			}, nil
		})()

		mockFMNormal := &mockFileManager{
			episodes: []files.EpisodeStruct{},
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				ExcludedLists:         []string{"Blacklist"},
				DownloadStatuses:      []string{"CURRENT", "REPEATING"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverNormal := &Server{State: state, FileManager: mockFMNormal}
		handlerNormal := handleAnimes(serverNormal)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()

		handlerNormal(w, req)

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		animes := response.Data.([]interface{})
		if len(animes) != 1 {
			t.Fatalf("Expected 1 anime, got %d", len(animes))
		}

		animeData := animes[0].(map[string]interface{})
		if _, exists := animeData["is_blacklisted"]; exists {
			t.Error("Expected is_blacklisted to be absent for non-blacklisted anime")
		}
	})

	t.Run("Downloaded anime whose status fell out of allowed sets stays visible and gets refreshed", func(t *testing.T) {
		orphanInfoBody := `{"data":{"Page":{"mediaList":[{"id":700999,"status":"COMPLETED","progress":10,"customLists":{"Blacklist":false},"media":{"id":999,"episodes":12,"format":"TV","status":"HIATUS","title":{"english":"Old Anime","romaji":"Old Anime"},"coverImage":{"large":"http://cover","medium":""},"airingSchedule":{"nodes":[]}}}]}}}`
		emptyListBody := `{"data":{"Page":{"mediaList":[]}}}`
		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			body := readBody(req)
			respBody := emptyListBody
			if strings.Contains(body, "mediaId") {
				respBody = orphanInfoBody
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(respBody))}, nil
		})()

		mockFMOrphan := &mockFileManager{
			episodes: []files.EpisodeStruct{
				{EpisodeNumber: 1, AnimeID: 999, AnimeName: "Old Anime", AnimeTotalEpisodes: 12, EpisodeName: "Old Anime - Episode 1", DownloadDate: time.Now()},
			},
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverOrphan := &Server{State: state, FileManager: mockFMOrphan}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverOrphan)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		animes, ok := response.Data.([]interface{})
		if !ok || len(animes) != 1 {
			t.Fatalf("expected 1 anime (orphan stays visible), got %v", response.Data)
		}

		animeData := animes[0].(map[string]interface{})
		if animeData["anime_id"].(float64) != 999 {
			t.Errorf("expected anime_id 999, got %v", animeData["anime_id"])
		}
		if animeData["cover_image"] != "http://cover" {
			t.Errorf("expected refreshed cover_image, got %v", animeData["cover_image"])
		}
		if animeData["episodes_watched"].(float64) != 10 {
			t.Errorf("expected refreshed episodes_watched=10, got %v", animeData["episodes_watched"])
		}
	})

	// Regression of the inverse shape of the storm below: even with the list OK, one request
	// per orphan every 30s poll per tab was the biggest consumer of the AniList budget
	// (decisions.md #65) — and its burst limiter punishes exactly that shape.
	t.Run("Orphan refresh asks AniList once for all orphans, not once per anime", func(t *testing.T) {
		var batchCalls int32
		var sentIDs string
		emptyListBody := `{"data":{"Page":{"mediaList":[]}}}`
		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			body := readBody(req)
			if !strings.Contains(body, "mediaId_in") {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(emptyListBody))}, nil
			}
			atomic.AddInt32(&batchCalls, 1)
			sentIDs = body
			entries := `{"data":{"Page":{"mediaList":[
				{"id":1,"status":"COMPLETED","progress":12,"customLists":{},"media":{"id":991,"episodes":12,"format":"TV","status":"FINISHED","title":{"romaji":"A"},"coverImage":{"large":"http://a","medium":""},"airingSchedule":{"nodes":[]}}},
				{"id":2,"status":"COMPLETED","progress":12,"customLists":{},"media":{"id":992,"episodes":12,"format":"TV","status":"FINISHED","title":{"romaji":"B"},"coverImage":{"large":"http://b","medium":""},"airingSchedule":{"nodes":[]}}},
				{"id":3,"status":"COMPLETED","progress":12,"customLists":{},"media":{"id":993,"episodes":12,"format":"TV","status":"FINISHED","title":{"romaji":"C"},"coverImage":{"large":"http://c","medium":""},"airingSchedule":{"nodes":[]}}}
			]}}}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(entries))}, nil
		})()

		var episodes []files.EpisodeStruct
		for _, id := range []int{991, 992, 993} {
			episodes = append(episodes, files.EpisodeStruct{
				EpisodeNumber: 1, AnimeID: id, AnimeName: "Downloaded", EpisodeName: "Downloaded - Episode 1",
				DownloadDate: time.Now(),
			})
		}

		serverBatch := &Server{State: state, FileManager: &mockFileManager{
			episodes: episodes,
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverBatch)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if n := atomic.LoadInt32(&batchCalls); n != 1 {
			t.Fatalf("want 1 request for the 3 orphans, got %d", n)
		}
		for _, id := range []string{"991", "992", "993"} {
			if !strings.Contains(sentIDs, id) {
				t.Fatalf("the batch must carry every orphan id, missing %s: %s", id, sentIDs)
			}
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		animes, ok := response.Data.([]interface{})
		if !ok || len(animes) != 3 {
			t.Fatalf("the 3 orphans must stay visible, got %v", response.Data)
		}
		for _, a := range animes {
			if a.(map[string]interface{})["cover_image"] == "" {
				t.Fatalf("orphan without cover: the batch did not reach it: %v", a)
			}
		}
	})

	// Regression: when the list fetch fails, fetchAniListEntries returns nil and the
	// "covered" set is empty — which made EVERY anime with a downloaded episode become
	// an orphan and fire an individual GetAnimeInfo per anime, on every /api/v1/animes poll.
	// That multiplied a transient AniList failure into hundreds of requests per minute,
	// leading to 429; the 429 made the list fail again, trapping the daemon in the cycle.
	t.Run("List fetch failure does not trigger per-anime refresh storm", func(t *testing.T) {
		var perAnimeCalls int32
		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(readBody(req), "mediaId") {
				atomic.AddInt32(&perAnimeCalls, 1)
			}
			// AniList is down / rate-limiting: every query fails.
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
		})()

		var episodes []files.EpisodeStruct
		for i := 0; i < 16; i++ {
			episodes = append(episodes, files.EpisodeStruct{
				EpisodeNumber: i, AnimeID: 900 + i, AnimeName: "Downloaded Anime",
				EpisodeName: "Downloaded Anime - Episode 1", DownloadDate: time.Now(),
			})
		}

		serverStorm := &Server{State: state, FileManager: &mockFileManager{
			episodes: episodes,
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverStorm)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 (local data still serves), got %d", w.Code)
		}
		if n := atomic.LoadInt32(&perAnimeCalls); n != 0 {
			t.Errorf("want 0 requests per anime when the list failed, got %d", n)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if animes, ok := response.Data.([]interface{}); !ok || len(animes) != 16 {
			t.Fatalf("downloaded animes must stay visible, got %v", response.Data)
		}
	})

	t.Run("Orphan refresh failure keeps anime visible without refreshed fields", func(t *testing.T) {
		emptyListBody := `{"data":{"Page":{"mediaList":[]}}}`
		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			body := readBody(req)
			if strings.Contains(body, "mediaId") {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("error"))}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(emptyListBody))}, nil
		})()

		mockFMOrphanFail := &mockFileManager{
			episodes: []files.EpisodeStruct{
				{EpisodeNumber: 1, AnimeID: 998, AnimeName: "Failing Refresh Anime", AnimeTotalEpisodes: 12, EpisodeName: "Failing Refresh Anime - Episode 1", DownloadDate: time.Now()},
			},
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverOrphanFail := &Server{State: state, FileManager: mockFMOrphanFail}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverOrphanFail)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 even when orphan refresh fails, got %d", w.Code)
		}

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		animes, ok := response.Data.([]interface{})
		if !ok || len(animes) != 1 {
			t.Fatalf("expected anime to stay visible despite refresh failure, got %v", response.Data)
		}
		animeData := animes[0].(map[string]interface{})
		if animeData["name"] != "Failing Refresh Anime" {
			t.Errorf("expected name from episodes.json to stand, got %v", animeData["name"])
		}
	})

	t.Run("Anime with no downloaded episodes and disallowed media status is not merged in", func(t *testing.T) {
		anilistBody := `{"data":{"Page":{"mediaList":[{"id":55,"status":"CURRENT","progress":0,"customLists":{},"media":{"format":"TV","status":"CANCELLED","episodes":12,"title":{"english":"Cancelled Anime","romaji":"Cancelled Anime"},"airingSchedule":{"nodes":[]}}}]}}}`
		defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(anilistBody))}, nil
		})()

		mockFMCancelled := &mockFileManager{
			episodes: []files.EpisodeStruct{},
			configs: &files.Config{
				AnilistUsernames:      []string{"testuser"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}
		serverCancelled := &Server{State: state, FileManager: mockFMCancelled}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverCancelled)(w, req)

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("expected data to be an array")
		}
		if len(animes) != 0 {
			t.Fatalf("expected CANCELLED anime with no downloads to be filtered out, got %d", len(animes))
		}
	})

	// Regression: MediaList.Id is the ENTRY id, which is per account — the same anime in two
	// accounts arrives with two ids. While identity was that id, the second account's entry
	// did not match the one already in animeMap and was inserted again by name: the anime
	// showed up twice in the frontend, even though it downloaded once. Identity is now Media.Id.
	t.Run("Same anime in two accounts is listed once", func(t *testing.T) {
		entry := func(id int, progress int) string {
			return fmt.Sprintf(`{"id":%d,"status":"CURRENT","progress":%d,"customLists":{},"media":{"id":194829,"format":"TV","status":"RELEASING","episodes":12,"title":{"english":"Two Account Anime","romaji":"Two Account Anime"},"coverImage":{"large":"http://cover","medium":""},"synonyms":[],"relations":{"edges":[]},"airingSchedule":{"nodes":[]}}}`, id, progress)
		}
		defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
			body := readBody(req)
			if strings.Contains(body, "mediaId") {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("no per-anime call expected"))}, nil
			}
			// Each account returns the SAME media with a different entry id.
			id, progress := 111, 5
			if strings.Contains(body, "accountB") {
				id, progress = 222, 0
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
				`{"data":{"Page":{"mediaList":[` + entry(id, progress) + `]}}}`))}, nil
		})()

		serverDup := &Server{State: state, FileManager: &mockFileManager{
			// Downloaded episode anchored on account B's entry.
			episodes: []files.EpisodeStruct{{
				EpisodeNumber: 1, AnimeID: 194829, AnimeName: "Two Account Anime",
				EpisodeName: "Two Account Anime - Episode 1", DownloadDate: time.Now(),
			}},
			configs: &files.Config{
				AnilistUsernames:      []string{"accountA", "accountB"},
				DownloadStatuses:      []string{"CURRENT"},
				DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/animes", nil)
		w := httptest.NewRecorder()
		handleAnimes(serverDup)(w, req)

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		animes, ok := response.Data.([]interface{})
		if !ok {
			t.Fatal("expected data to be an array")
		}
		if len(animes) != 1 {
			t.Fatalf("the same anime in 2 accounts must appear once, got %d: %v", len(animes), response.Data)
		}
		animeData := animes[0].(map[string]interface{})
		if animeData["anime_id"].(float64) != 194829 {
			t.Errorf("identity is the MEDIA id (194829), got %v", animeData["anime_id"])
		}
		if animeData["episodes_downloaded"].(float64) != 1 {
			t.Errorf("the downloaded episode must still be counted, got %v", animeData["episodes_downloaded"])
		}
	})
}

func TestCountPendingEpisodes(t *testing.T) {
	tests := []struct {
		name       string
		info       AnimeInfo
		downloaded map[int]bool
		want       int
	}{
		{"watched up to the last released is not pending", AnimeInfo{EpisodesWatched: 8, EpisodesReleased: 8}, nil, 0},
		{"only counts what is past progress", AnimeInfo{EpisodesWatched: 5, EpisodesReleased: 10}, map[int]bool{6: true, 7: true}, 3},
		{"watched saved on disk does not hide the pending count", AnimeInfo{EpisodesWatched: 5, EpisodesReleased: 10}, map[int]bool{4: true, 5: true, 6: true}, 4},
		{"a hole in the middle counts", AnimeInfo{EpisodesWatched: 0, EpisodesReleased: 3}, map[int]bool{1: true, 3: true}, 1},
		{"progress above released does not go negative", AnimeInfo{EpisodesWatched: 12, EpisodesReleased: 10}, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countPendingEpisodes(&tt.info, tt.downloaded); got != tt.want {
				t.Fatalf("countPendingEpisodes = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestNextAiringAt covers the field that feeds the "Next episode in X" card: it comes from
// AniList's nextAiringEpisode, which is NIL on a finished anime and in gaps between seasons.
// Both merge paths (new entry and an entry that already had a downloaded episode) need to
// fill the same field — that is why the read became a helper of its own.
func TestNextAiringAt(t *testing.T) {
	str := func(s string) *string { return &s }
	entry := func(id int, next *anilist.AiringNode) anilist.MediaList {
		return anilist.MediaList{
			Media: anilist.Media{
				Id:                id,
				Title:             anilist.Title{English: str("Anime")},
				Status:            anilist.MediaStatusReleasing,
				NextAiringEpisode: next,
			},
		}
	}

	t.Run("new entry gets the timestamp", func(t *testing.T) {
		animeMap := map[string]*AnimeInfo{}
		mergeAniListAnimes(animeMap, []anilist.MediaList{entry(1, &anilist.AiringNode{Episode: 5, AiringAt: 1740394800})}, nil)

		if got := animeMap["Anime"].NextAiringAt; got != 1740394800 {
			t.Errorf("next_airing_at = %d, want 1740394800", got)
		}
	})

	t.Run("existing entry is also updated", func(t *testing.T) {
		existing := &AnimeInfo{AnimeID: 1, Name: "Anime", EpisodesDownloaded: 3}
		animeMap := map[string]*AnimeInfo{"Anime": existing}
		mergeAniListAnimes(animeMap, []anilist.MediaList{entry(1, &anilist.AiringNode{Episode: 5, AiringAt: 1740394800})}, nil)

		if existing.NextAiringAt != 1740394800 {
			t.Errorf("next_airing_at = %d, want 1740394800", existing.NextAiringAt)
		}
	})

	t.Run("nil nextAiringEpisode comes out as zero and drops from JSON", func(t *testing.T) {
		animeMap := map[string]*AnimeInfo{}
		mergeAniListAnimes(animeMap, []anilist.MediaList{entry(1, nil)}, nil)

		if got := animeMap["Anime"].NextAiringAt; got != 0 {
			t.Errorf("next_airing_at = %d, want 0 for an anime with no scheduled episode", got)
		}
		body, err := json.Marshal(*animeMap["Anime"])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(body), "next_airing_at") {
			t.Errorf("omitempty should drop the field, got %s", body)
		}
	})
}

// TestAltNames covers the list that feeds dashboard search: the card shows a single title
// (English, when it exists), but the user may search by romaji, kanji or a synonym — all
// must come out in the payload, without repeating the name already in Name.
func TestAltNames(t *testing.T) {
	str := func(s string) *string { return &s }
	media := anilist.Media{
		Title:    anilist.Title{English: str("Attack on Titan"), Romaji: str("Shingeki no Kyojin"), Native: str("進撃の巨人")},
		Synonyms: []string{"AoT", "  ", "attack on titan", "Shingeki no Kyojin"},
	}

	got := altNames(media, "Attack on Titan")
	want := []string{"Shingeki no Kyojin", "進撃の巨人", "AoT"}
	if len(got) != len(want) {
		t.Fatalf("altNames = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("altNames = %q, want %q", got, want)
		}
	}

	if got := altNames(anilist.Media{Title: anilist.Title{Romaji: str("Solo")}}, "Solo"); len(got) != 0 {
		t.Fatalf("a unique title must not become an alt_name: %q", got)
	}
}

// TestMergeAniListAnimesAltNames guarantees both merge paths fill alt_names — and the existing
// entry path is the one that matters: an already-downloaded anime is the common dashboard case.
func TestMergeAniListAnimesAltNames(t *testing.T) {
	str := func(s string) *string { return &s }
	entry := anilist.MediaList{Media: anilist.Media{
		Id:       1,
		Title:    anilist.Title{English: str("Anime"), Native: str("アニメ")},
		Status:   anilist.MediaStatusReleasing,
		Synonyms: []string{"Anime the Animation"},
	}}

	animeMap := map[string]*AnimeInfo{}
	mergeAniListAnimes(animeMap, []anilist.MediaList{entry}, nil)
	if len(animeMap["Anime"].AltNames) != 2 {
		t.Errorf("new entry missing alt_names: %q", animeMap["Anime"].AltNames)
	}

	existing := &AnimeInfo{AnimeID: 1, Name: "Anime", EpisodesDownloaded: 3}
	mergeAniListAnimes(map[string]*AnimeInfo{"Anime": existing}, []anilist.MediaList{entry}, nil)
	if len(existing.AltNames) != 2 {
		t.Errorf("existing entry missing alt_names: %q", existing.AltNames)
	}
}
