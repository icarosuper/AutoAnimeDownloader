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

// MockHTTPClient implementa HTTPClient para testes
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

	// Default response
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

	// Default response
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

func TestTorrentModuleFixesInvalidFolderName(t *testing.T) {
	// Torrent module must fix invalid folder names to prevent errors when adding torrents
	// Example: "Anime: Brabo" -> "Anime Brabo"

	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock successful torrent addition
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/add", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})

	// Mock successful torrent list response
	torrentsJSON := `[{"hash": "abc123", "name": "Episode 01", "magnet_uri": "magnet:test", "save_path": "/path", "content_path": "/path"}]`
	mockClient.SetGetResponse("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(torrentsJSON)),
	})

	// Test with anime name containing invalid characters
	hash := service.DownloadTorrent("magnet:test", "/save/path", "Anime: Brabo", "Episode 01")

	if hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", hash)
	}

	// Verify that the request was made with sanitized folder name
	history := mockClient.GetRequestHistory()
	if len(history) != 2 {
		t.Errorf("Expected 2 requests, got %d", len(history))
	}

	// Check POST request for adding torrent
	postReq := history[0]
	if postReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", postReq.Method)
	}
	if !strings.Contains(postReq.URL, "/add") {
		t.Errorf("Expected URL to contain '/add', got %s", postReq.URL)
	}

	// Verify that the savepath was sanitized (removed the colon)
	expectedSavePath := "/save/path/Anime Brabo"
	if postReq.Data.Get("savepath") != expectedSavePath {
		t.Errorf("Expected savepath '%s', got '%s'", expectedSavePath, postReq.Data.Get("savepath"))
	}
}

func TestTorrentService_DownloadTorrent_Success(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock successful torrent addition
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/add", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})

	// Mock successful torrent list response
	torrentsJSON := `[{"hash": "abc123", "name": "Episode 01", "magnet_uri": "magnet:test", "save_path": "/path", "content_path": "/path"}]`
	mockClient.SetGetResponse("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(torrentsJSON)),
	})

	hash := service.DownloadTorrent("magnet:test", "/save/path", "Anime", "Episode 01")

	if hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", hash)
	}

	// Verify requests were made
	history := mockClient.GetRequestHistory()
	if len(history) != 2 {
		t.Errorf("Expected 2 requests, got %d", len(history))
	}

	// Check POST request for adding torrent
	postReq := history[0]
	if postReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", postReq.Method)
	}
	if !strings.Contains(postReq.URL, "/add") {
		t.Errorf("Expected URL to contain '/add', got %s", postReq.URL)
	}
}

func TestTorrentService_DownloadTorrent_AddTorrentFails(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock failed torrent addition
	mockClient.SetPostError("http://localhost:8080/api/v2/torrents/add", fmt.Errorf("connection failed"))

	hash := service.DownloadTorrent("magnet:test", "/save/path", "Anime", "Episode 01")

	if hash != "" {
		t.Errorf("Expected empty hash on failure, got '%s'", hash)
	}
}

func TestTorrentService_DownloadTorrent_GetHashFails(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock successful torrent addition
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/add", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})

	// Mock failed torrent list response
	mockClient.SetGetError("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", fmt.Errorf("connection failed"))

	hash := service.DownloadTorrent("magnet:test", "/save/path", "Anime", "Episode 01")

	if hash != "" {
		t.Errorf("Expected empty hash on failure, got '%s'", hash)
	}
}

func TestTorrentService_DeleteTorrents_Success(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock successful deletion
	mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/delete", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	})

	err := service.DeleteTorrents([]string{"hash1", "hash2"})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify request was made
	history := mockClient.GetRequestHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 request, got %d", len(history))
	}

	postReq := history[0]
	if postReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", postReq.Method)
	}
	if !strings.Contains(postReq.URL, "/delete") {
		t.Errorf("Expected URL to contain '/delete', got %s", postReq.URL)
	}
	if postReq.Data.Get("hashes") != "hash1|hash2" {
		t.Errorf("Expected hashes 'hash1|hash2', got '%s'", postReq.Data.Get("hashes"))
	}
	if postReq.Data.Get("deleteFiles") != "true" {
		t.Errorf("Expected deleteFiles 'true', got '%s'", postReq.Data.Get("deleteFiles"))
	}
}

func TestTorrentService_DeleteTorrents_EmptyHashes(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	err := service.DeleteTorrents([]string{})

	if err != nil {
		t.Errorf("Expected no error for empty hashes, got %v", err)
	}

	// Verify no request was made
	history := mockClient.GetRequestHistory()
	if len(history) != 0 {
		t.Errorf("Expected no requests for empty hashes, got %d", len(history))
	}
}

func TestTorrentService_GetDownloadedTorrents_Success(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock successful torrent list response
	torrentsJSON := `[
		{"hash": "hash1", "name": "Episode 01", "magnet_uri": "magnet:test1", "save_path": "/path1", "content_path": "/path1"},
		{"hash": "hash2", "name": "Episode 02", "magnet_uri": "magnet:test2", "save_path": "/path2", "content_path": "/path2"}
	]`
	mockClient.SetGetResponse("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(torrentsJSON)),
	})

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

func TestTorrentService_GetDownloadedTorrents_Failure(t *testing.T) {
	mockClient := NewMockHTTPClient()
	service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

	// Mock failed response
	mockClient.SetGetError("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", fmt.Errorf("connection failed"))

	torrents, err := service.GetDownloadedTorrents()

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if torrents != nil {
		t.Errorf("Expected nil torrents on error, got %v", torrents)
	}
}

// Testes para métodos privados foram removidos pois não são acessíveis
// Os métodos privados são testados indiretamente através dos métodos públicos

func TestSanitizeFolderName(t *testing.T) {
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
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			// Create a fresh mock client for each test case
			mockClient := NewMockHTTPClient()
			service := torrents.NewTorrentService(mockClient, "http://localhost:8080")

			// Mock successful torrent addition
			mockClient.SetPostResponse("http://localhost:8080/api/v2/torrents/add", &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("")),
			})

			// Mock successful torrent list response
			torrentsJSON := `[{"hash": "abc123", "name": "Episode 01", "magnet_uri": "magnet:test", "save_path": "/path", "content_path": "/path"}]`
			mockClient.SetGetResponse("http://localhost:8080/api/v2/torrents/info?category=autoAnimeDownloader", &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(torrentsJSON)),
			})
			
			service.DownloadTorrent("magnet:test", "/save/path", tc.input, "Episode 01")

			// Verify that the request was made with sanitized folder name
			history := mockClient.GetRequestHistory()
			if len(history) < 1 {
				t.Fatalf("Expected at least 1 request, got %d", len(history))
			}

			postReq := history[0]
			expectedSavePath := "/save/path/" + tc.expected
			if postReq.Data.Get("savepath") != expectedSavePath {
				t.Errorf("Expected savepath '%s', got '%s'", expectedSavePath, postReq.Data.Get("savepath"))
			}
		})
	}
}
