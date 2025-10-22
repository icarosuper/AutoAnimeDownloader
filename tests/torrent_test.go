package tests

import (
	"AutoAnimeDownloader/modules/torrents"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type MockHTTPClient struct {
	GetResponses   map[string]*http.Response
	PostResponses  map[string]*http.Response
	GetErrors      map[string]error
	PostErrors     map[string]error
	RequestHistory []MockRequest
}

type MockRequest struct {
	Method string
	URL    string
	Data   url.Values
}

func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		GetResponses:   make(map[string]*http.Response),
		PostResponses:  make(map[string]*http.Response),
		GetErrors:      make(map[string]error),
		PostErrors:     make(map[string]error),
		RequestHistory: make([]MockRequest, 0),
	}
}

func (m *MockHTTPClient) Get(url string) (*http.Response, error) {
	m.RequestHistory = append(m.RequestHistory, MockRequest{Method: "GET", URL: url})

	if err, exists := m.GetErrors[url]; exists {
		return nil, err
	}

	if resp, exists := m.GetResponses[url]; exists {
		return resp, nil
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("[]")),
	}, nil
}

func (m *MockHTTPClient) PostForm(url string, data url.Values) (*http.Response, error) {
	m.RequestHistory = append(m.RequestHistory, MockRequest{Method: "POST", URL: url, Data: data})

	if err, exists := m.PostErrors[url]; exists {
		return nil, err
	}

	if resp, exists := m.PostResponses[url]; exists {
		return resp, nil
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (m *MockHTTPClient) SetGetResponse(url string, response *http.Response) {
	m.GetResponses[url] = response
}

func (m *MockHTTPClient) SetPostResponse(url string, response *http.Response) {
	m.PostResponses[url] = response
}

func (m *MockHTTPClient) SetGetError(url string, err error) {
	m.GetErrors[url] = err
}

func (m *MockHTTPClient) SetPostError(url string, err error) {
	m.PostErrors[url] = err
}

func (m *MockHTTPClient) GetRequestHistory() []MockRequest {
	return m.RequestHistory
}

func (m *MockHTTPClient) ClearHistory() {
	m.RequestHistory = make([]MockRequest, 0)
}

const savePath = "/save/path/"

func setupMockService() (*MockHTTPClient, *torrents.TorrentService) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080", savePath)
	return mockClient, service
}

func mockSuccessfulTorrentAdd(mockClient *MockHTTPClient) {
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/add", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})
}

func mockSuccessfulTorrentList(mockClient *MockHTTPClient, torrentsJSON string) {
	mockClient.SetGetResponse("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(torrentsJSON)),
	})
}

func mockFailedTorrentAdd(mockClient *MockHTTPClient, err error) {
	mockClient.SetPostError("http://localhost:8080/api/v2/torrents/add", err)
}

func mockFailedTorrentList(mockClient *MockHTTPClient, err error) {
	mockClient.SetGetError("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", err)
}

func mockSuccessfulTorrentDelete(mockClient *MockHTTPClient) {
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/delete", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})
}

func verifyRequestCount(t *testing.T, mockClient *MockHTTPClient, expectedCount int) {
	history := mockClient.GetRequestHistory()
	if len(history) != expectedCount {
		t.Errorf("Expected %d requests, got %d", expectedCount, len(history))
	}
}

func verifyPostRequest(t *testing.T, mockClient *MockHTTPClient, expectedURLContains string) {
	history := mockClient.GetRequestHistory()
	if len(history) == 0 {
		t.Fatal("No requests found in history")
	}

	postReq := history[0]
	if postReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", postReq.Method)
	}
	if !strings.Contains(postReq.URL, expectedURLContains) {
		t.Errorf("Expected URL to contain '%s', got %s", expectedURLContains, postReq.URL)
	}
}

func getStandardTorrentsJSON() string {
	return `[{"hash": "abc123", "name": "Episode 01", "magnet_uri": "magnet:test", "save_path": "/path", "content_path": "/path"}]`
}

func TestTorrentService_CanDownloadTorrent_WithValidMagnet(t *testing.T) {
	mockClient, service := setupMockService()

	mockSuccessfulTorrentAdd(mockClient)
	mockSuccessfulTorrentList(mockClient, getStandardTorrentsJSON())

	hash := service.DownloadTorrent("magnet:test", "Anime", "Episode 01")

	if hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", hash)
	}

	verifyRequestCount(t, mockClient, 2)
	verifyPostRequest(t, mockClient, "/add")
}

func TestTorrentService_CannotDownloadTorrent_WithInvalidMagnet(t *testing.T) {
	mockClient, service := setupMockService()

	mockFailedTorrentAdd(mockClient, fmt.Errorf("connection failed"))

	hash := service.DownloadTorrent("magnet:test", "Anime", "Episode 01")

	if hash != "" {
		t.Errorf("Expected empty hash on failure, got '%s'", hash)
	}
}

func TestTorrentService_CannotDownloadTorrent_WithInvalidEpisodeName(t *testing.T) {
	mockClient, service := setupMockService()

	mockSuccessfulTorrentAdd(mockClient)
	mockFailedTorrentList(mockClient, fmt.Errorf("connection failed"))

	hash := service.DownloadTorrent("magnet:test", "Anime", "Episode 01")

	if hash != "" {
		t.Errorf("Expected empty hash on failure, got '%s'", hash)
	}
}

func TestTorrentService_CanDeleteTorrents_WithValidHashes(t *testing.T) {
	mockClient, service := setupMockService()

	mockSuccessfulTorrentDelete(mockClient)

	err := service.DeleteTorrents([]string{"hash1", "hash2"})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	verifyRequestCount(t, mockClient, 1)
	verifyPostRequest(t, mockClient, "/delete")

	history := mockClient.GetRequestHistory()
	postReq := history[0]
	if postReq.Data.Get("hashes") != "hash1|hash2" {
		t.Errorf("Expected hashes 'hash1|hash2', got '%s'", postReq.Data.Get("hashes"))
	}
	if postReq.Data.Get("deleteFiles") != "true" {
		t.Errorf("Expected deleteFiles 'true', got '%s'", postReq.Data.Get("deleteFiles"))
	}
}

func TestTorrentService_CannotDeleteTorrents_WithEmptyHashes(t *testing.T) {
	mockClient, service := setupMockService()

	err := service.DeleteTorrents([]string{})

	if err != nil {
		t.Errorf("Expected no error for empty hashes, got %v", err)
	}

	verifyRequestCount(t, mockClient, 0)
}

func TestTorrentService_CanGetDownloadedTorrents_WithValidResponse(t *testing.T) {
	mockClient, service := setupMockService()

	torrentsJSON := `[
		{"hash": "hash1", "name": "Episode 01", "magnet_uri": "magnet:test1", "save_path": "/path1", "content_path": "/path1"},
		{"hash": "hash2", "name": "Episode 02", "magnet_uri": "magnet:test2", "save_path": "/path2", "content_path": "/path2"}
	]`
	mockSuccessfulTorrentList(mockClient, torrentsJSON)

	torrents, err := service.GetDownloadedTorrents()

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(torrents) != 2 {
		t.Errorf("Expected 2 torrents, got %d", len(torrents))
	}

	if torrents[0].Hash != "hash1" {
		t.Errorf("Expected first torrent hash 'hash1', got '%s'", torrents[0].Hash)
	}
	if torrents[1].Hash != "hash2" {
		t.Errorf("Expected second torrent hash 'hash2', got '%s'", torrents[1].Hash)
	}
}

func TestTorrentService_CannotGetDownloadedTorrents_WithInvalidResponse(t *testing.T) {
	mockClient, service := setupMockService()

	mockFailedTorrentList(mockClient, fmt.Errorf("connection failed"))

	torrents, err := service.GetDownloadedTorrents()

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if torrents != nil {
		t.Errorf("Expected nil torrents on error, got %v", torrents)
	}
}

func TestTorrentService_SanitizesFolderName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Anime: Brabo", "Anime Brabo"},
		{"Anime<Test>", "AnimeTest"},
		{"Anime|Test", "AnimeTest"},
		{"Anime?Test", "AnimeTest"},
		{"Anime*Test", "AnimeTest"},
		{"Anime\"Test\"", "AnimeTest"},
		{"Anime\\Test", "AnimeTest"},
		{"Anime/Test", "AnimeTest"},
		{"  Anime  Test  ", "Anime Test"},
		{"Anime:::Test", "AnimeTest"},
		{"Ranma 1/2", "Ranma 12"}, // Real bug case
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			mockClient, service := setupMockService()

			mockSuccessfulTorrentAdd(mockClient)
			mockSuccessfulTorrentList(mockClient, getStandardTorrentsJSON())

			service.DownloadTorrent("magnet:test", tc.input, "Episode 01")

			history := mockClient.GetRequestHistory()
			if len(history) < 1 {
				t.Fatalf("Expected at least 1 request, got %d", len(history))
			}

			postReq := history[0]
			expectedSavePath := savePath + tc.expected
			if postReq.Data.Get("savepath") != expectedSavePath {
				t.Errorf("Expected savepath '%s', got '%s'", expectedSavePath, postReq.Data.Get("savepath"))
			}
		})
	}
}
