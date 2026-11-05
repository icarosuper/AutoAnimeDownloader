package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the HTTP client the CLI uses to talk to the daemon API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// APIError is a daemon envelope with success=false. The CLI JSON error uses Code and Message.
type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error: %s - %s", e.Code, e.Message)
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// slowTimeout bounds the calls that search Nyaa inside the request: RunAnimeDebug pages Nyaa and
// falls back to one search per episode, which takes minutes on a long series. The default 30s is
// for calls that answer from local state.
const slowTimeout = 10 * time.Minute

// sendSlow is send with slowTimeout instead of the client's default.
func (c *Client) sendSlow(method, path string, body, target interface{}) error {
	slow := *c.httpClient
	slow.Timeout = slowTimeout
	resp, err := c.doRequestWith(&slow, method, path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, target)
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	return c.doRequestWith(c.httpClient, method, path, body)
}

func (c *Client) doRequestWith(httpClient *http.Client, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

func (c *Client) parseResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp SuccessResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !apiResp.Success {
		if apiResp.Error != nil {
			return &APIError{Code: apiResp.Error.Code, Message: apiResp.Error.Message}
		}
		return &APIError{Code: "API_ERROR", Message: "API returned error response"}
	}

	if target != nil && apiResp.Data != nil {
		dataBytes, err := json.Marshal(apiResp.Data)
		if err != nil {
			return fmt.Errorf("failed to marshal response data: %w", err)
		}
		if err := json.Unmarshal(dataBytes, target); err != nil {
			return fmt.Errorf("failed to unmarshal response data: %w", err)
		}
	}

	return nil
}

func (c *Client) get(path string, target interface{}) error {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, target)
}

func (c *Client) send(method, path string, body, target interface{}) error {
	resp, err := c.doRequest(method, path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, target)
}

func (c *Client) GetStatus() (*StatusResponse, error) {
	var statusResp StatusResponse
	if err := c.get("/api/v1/status", &statusResp); err != nil {
		return nil, err
	}
	return &statusResp, nil
}

func (c *Client) GetConfig() (*files.Config, error) {
	var config files.Config
	if err := c.get("/api/v1/config", &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Client) UpdateConfig(config *files.Config) error {
	return c.send(http.MethodPut, "/api/v1/config", config, nil)
}

func (c *Client) GetAnimes() ([]AnimeInfo, error) {
	var animes []AnimeInfo
	if err := c.get("/api/v1/animes", &animes); err != nil {
		return nil, err
	}
	return animes, nil
}

func (c *Client) TriggerCheck() error {
	return c.send(http.MethodPost, "/api/v1/check", nil, nil)
}

func (c *Client) StartLoop() error {
	return c.send(http.MethodPost, "/api/v1/daemon/start", nil, nil)
}

func (c *Client) StopLoop() error {
	return c.send(http.MethodPost, "/api/v1/daemon/stop", nil, nil)
}

func (c *Client) GetLastCheck() (*daemon.CheckReport, error) {
	var report daemon.CheckReport
	if err := c.get("/api/v1/last-check", &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func (c *Client) GetPriorityDefaults() (nyaa.Priorities, error) {
	var priorities nyaa.Priorities
	if err := c.get("/api/v1/config/priorities/defaults", &priorities); err != nil {
		return nyaa.Priorities{}, err
	}
	return priorities, nil
}

func (c *Client) SearchAniList(term string, includeUnreleased bool) ([]AniListSearchResult, error) {
	q := url.Values{}
	q.Set("q", term)
	if includeUnreleased {
		q.Set("include_unreleased", "true")
	}
	var results []AniListSearchResult
	if err := c.get("/api/v1/anilist/search?"+q.Encode(), &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Client) GetTorrentCandidates(id int, query string) (*TorrentCandidatesResponse, error) {
	path := "/api/v1/animes/" + strconv.Itoa(id) + "/torrent-candidates"
	if query != "" {
		q := url.Values{}
		q.Set("q", query)
		path += "?" + q.Encode()
	}
	var out TorrentCandidatesResponse
	if err := c.get(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AddStandaloneAnime(req StandaloneAnimeRequest) (*StandaloneAnimeAddResponse, error) {
	var out StandaloneAnimeAddResponse
	if err := c.send(http.MethodPost, "/api/v1/standalone-animes", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RemoveStandaloneAnime(id int, deleteEpisodes bool) error {
	path := "/api/v1/standalone-animes/" + strconv.Itoa(id)
	if deleteEpisodes {
		path += "?delete_episodes=true"
	}
	return c.send(http.MethodDelete, path, nil, nil)
}

func (c *Client) GetAnimeSettings(id int) (*files.AnimeSettings, error) {
	var settings files.AnimeSettings
	if err := c.get("/api/v1/animes/"+strconv.Itoa(id)+"/settings", &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (c *Client) UpdateAnimeSettings(id, progress int) error {
	body := map[string]int{"progress": progress}
	return c.send(http.MethodPut, "/api/v1/animes/"+strconv.Itoa(id)+"/settings", body, nil)
}

func (c *Client) GetAnimeEpisodes(id int) (*AnimeDetailResponse, error) {
	var detail AnimeDetailResponse
	if err := c.get("/api/v1/animes/"+strconv.Itoa(id)+"/episodes", &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (c *Client) DownloadEpisode(id, episode int) error {
	return c.send(http.MethodPost, episodePath(id, episode)+"/download", nil, nil)
}

func (c *Client) RedownloadEpisode(id, episode int) error {
	return c.send(http.MethodPost, episodePath(id, episode)+"/redownload", nil, nil)
}

func (c *Client) DeleteEpisode(id, episode int) error {
	return c.send(http.MethodDelete, episodePath(id, episode), nil, nil)
}

func (c *Client) ReleaseEpisode(id, episode int) error {
	return c.send(http.MethodPost, episodePath(id, episode)+"/release", nil, nil)
}

func (c *Client) ReplaceAnime(id int, magnet string) error {
	return c.send(http.MethodPost, "/api/v1/animes/"+strconv.Itoa(id)+"/replace", map[string]string{"magnet": magnet}, nil)
}

func (c *Client) ReplaceEpisode(id, episode int, magnet string) error {
	return c.send(http.MethodPost, episodePath(id, episode)+"/replace", map[string]string{"magnet": magnet}, nil)
}

func (c *Client) GetTorrents() ([]TorrentResponse, error) {
	var torrents []TorrentResponse
	if err := c.get("/api/v1/torrents", &torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

func (c *Client) GetTorrentFiles(hash string) ([]TorrentFileResponse, error) {
	var list []TorrentFileResponse
	if err := c.get("/api/v1/torrents/"+hash+"/files", &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (c *Client) PauseTorrent(hash string) error {
	return c.send(http.MethodPost, "/api/v1/torrents/"+hash+"/pause", nil, nil)
}

func (c *Client) ResumeTorrent(hash string) error {
	return c.send(http.MethodPost, "/api/v1/torrents/"+hash+"/resume", nil, nil)
}

func (c *Client) AnnounceTorrent(hash string) error {
	return c.send(http.MethodPost, "/api/v1/torrents/"+hash+"/announce", nil, nil)
}

func (c *Client) PrioritizeTorrent(hash string) error {
	return c.send(http.MethodPost, "/api/v1/torrents/"+hash+"/prioritize", nil, nil)
}

func (c *Client) PrioritizeTorrents(hashes []string) error {
	return c.send(http.MethodPost, "/api/v1/torrents/prioritize", PrioritizeRequest{Hashes: hashes}, nil)
}

func (c *Client) DeleteTorrent(hash string, keepData, block bool) error {
	path := "/api/v1/torrents/" + hash
	q := url.Values{}
	if keepData {
		q.Set("keep_data", "true")
	}
	if block {
		q.Set("block", "true")
	}
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return c.send(http.MethodDelete, path, nil, nil)
}

func (c *Client) TestWebhook(name string) error {
	return c.send(http.MethodPost, "/api/v1/notifications/webhooks/"+url.PathEscape(name)+"/test", nil, nil)
}

// PreviewWebhook renders preset with the test fire's sample variables; nothing is sent.
func (c *Client) PreviewWebhook(preset files.WebhookPreset) (*notifications.RenderedWebhook, error) {
	var rendered notifications.RenderedWebhook
	if err := c.send(http.MethodPost, "/api/v1/notifications/webhooks/preview", preset, &rendered); err != nil {
		return nil, err
	}
	return &rendered, nil
}

// RunAnimeDebug runs the daemon's debug pass for one anime and returns its summary. Nothing is
// downloaded; the trace lands in the daemon's log.
func (c *Client) RunAnimeDebug(id int) (*daemon.DebugSummary, error) {
	var summary daemon.DebugSummary
	if err := c.sendSlow(http.MethodPost, "/api/v1/animes/"+strconv.Itoa(id)+"/debug", nil, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func episodePath(id, episode int) string {
	return "/api/v1/animes/" + strconv.Itoa(id) + "/episodes/" + strconv.Itoa(episode)
}
