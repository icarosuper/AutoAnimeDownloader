package api

import (
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

