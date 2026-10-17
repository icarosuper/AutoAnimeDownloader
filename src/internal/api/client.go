package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client é o cliente HTTP para comunicação com a API do daemon
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
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

	resp, err := c.httpClient.Do(req)
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
			return fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
		}
		return fmt.Errorf("API returned error response")
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

func (c *Client) GetStatus() (*StatusResponse, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/status", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var statusResp StatusResponse
	if err := c.parseResponse(resp, &statusResp); err != nil {
		return nil, err
	}

	return &statusResp, nil
}

func (c *Client) GetConfig() (*files.Config, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/config", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var config files.Config
	if err := c.parseResponse(resp, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Client) UpdateConfig(config *files.Config) error {
	resp, err := c.doRequest(http.MethodPut, "/api/v1/config", config)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

func (c *Client) GetAnimes() ([]AnimeInfo, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/animes", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var animes []AnimeInfo
	if err := c.parseResponse(resp, &animes); err != nil {
		return nil, err
	}

	return animes, nil
}

func (c *Client) GetEpisodes() ([]files.EpisodeStruct, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/episodes", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var episodes []files.EpisodeStruct
	if err := c.parseResponse(resp, &episodes); err != nil {
		return nil, err
	}

	return episodes, nil
}

func (c *Client) TriggerCheck() error {
	resp, err := c.doRequest(http.MethodPost, "/api/v1/check", nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

func (c *Client) StartLoop() error {
	resp, err := c.doRequest(http.MethodPost, "/api/v1/daemon/start", nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

func (c *Client) StopLoop() error {
	resp, err := c.doRequest(http.MethodPost, "/api/v1/daemon/stop", nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

// LoadStandaloneAnimes returns the tracked standalone media ids, as the file stores them.
func (c *Client) LoadStandaloneAnimes() ([]int, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/standalone-animes", nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var ids []int
	if err := c.parseResponse(resp, &ids); err != nil {
		return nil, err
	}

	return ids, nil
}

// SaveStandaloneAnimes replaces the tracked list wholesale — the CLI's `standalones import`
// round-trips an export through it.
func (c *Client) SaveStandaloneAnimes(ids []int) error {
	resp, err := c.doRequest(http.MethodPut, "/api/v1/standalone-animes", ids)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

// AddStandaloneAnime starts tracking one anime and downloads what is available.
func (c *Client) AddStandaloneAnime(mediaID int) (int, error) {
	resp, err := c.doRequest(http.MethodPost, "/api/v1/standalone-animes", map[string]int{"media_id": mediaID})
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Added int `json:"added"`
	}
	if err := c.parseResponse(resp, &body); err != nil {
		return 0, err
	}

	return body.Added, nil
}

// RemoveStandaloneAnime stops tracking one anime. deleteEpisodes also removes the downloaded
// episode files; without it they stay on disk, marked manually managed.
func (c *Client) RemoveStandaloneAnime(mediaID int, deleteEpisodes bool) error {
	resp, err := c.doRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/standalone-animes/%d?delete_episodes=%t", mediaID, deleteEpisodes), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

// DeleteEpisode removes one episode's torrent, library hardlink and record, and blocks it.
func (c *Client) DeleteEpisode(animeID, episodeNumber int, keepData, block bool) error {
	resp, err := c.doRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/animes/%d/episodes/%d?keep_data=%t&block=%t", animeID, episodeNumber, keepData, block), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

// RedownloadEpisode drops the episode (if already on disk) and queues a fresh download.
func (c *Client) RedownloadEpisode(animeID, episodeNumber int) error {
	resp, err := c.doRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/animes/%d/episodes/%d/redownload", animeID, episodeNumber), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseResponse(resp, nil)
}

// BatchDeleteEpisodes removes N episodes of one anime in a single request.
func (c *Client) BatchDeleteEpisodes(animeID int, episodes []int, keepData, block bool) (int, error) {
	resp, err := c.doRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/animes/%d/episodes/batch-delete", animeID),
		map[string]any{"episodes": episodes, "keep_data": keepData, "block": block})
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Count int `json:"count"`
	}
	if err := c.parseResponse(resp, &body); err != nil {
		return 0, err
	}

	return body.Count, nil
}

// BatchRedownloadEpisodes re-downloads N episodes of one anime in a single request.
func (c *Client) BatchRedownloadEpisodes(animeID int, episodes []int) (int, error) {
	resp, err := c.doRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/animes/%d/episodes/batch-redownload", animeID),
		map[string]any{"episodes": episodes})
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Count int `json:"count"`
	}
	if err := c.parseResponse(resp, &body); err != nil {
		return 0, err
	}

	return body.Count, nil
}

// RunAnimeDebug reproduces the daemon's search/match pipeline for one anime over HTTP and
// returns the summary. Nothing is downloaded.
func (c *Client) RunAnimeDebug(animeID int) (*daemon.DebugSummary, error) {
	resp, err := c.doRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/debug/anime/%d", animeID), nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var summary daemon.DebugSummary
	if err := c.parseResponse(resp, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}
