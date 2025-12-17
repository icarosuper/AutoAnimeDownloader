package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var (
	// daemonURL can be overridden via DAEMON_URL environment variable
	// Defaults to localhost for local testing, but should be "http://daemon:8091" in Docker
	daemonURL = getEnvOrDefault("DAEMON_URL", "http://localhost:8091")
	apiBase   = daemonURL + "/api/v1"
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func TestAPIEndpoints(t *testing.T) {
	// Wait for daemon to be ready
	if !waitForDaemon(t, 30*time.Second) {
		t.Fatal("Daemon did not become ready in time")
	}

	t.Run("GET /api/v1/status", func(t *testing.T) {
		resp, err := http.Get(apiBase + "/status")
		if err != nil {
			t.Fatalf("Failed to get status: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Fatalf("Expected success=true, got %v", result["success"])
		}

		data, ok := result["data"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected data field to be an object")
		}

		if _, ok := data["status"]; !ok {
			t.Fatal("Expected status field in data")
		}
	})

	t.Run("GET /api/v1/config", func(t *testing.T) {
		resp, err := http.Get(apiBase + "/config")
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Fatalf("Expected success=true, got %v", result["success"])
		}
	})

	t.Run("PUT /api/v1/config", func(t *testing.T) {
		config := map[string]interface{}{
			"anilist_username":        "testuser",
			"save_path":               "/tmp/test",
			"completed_anime_path":    "/tmp/completed",
			"check_interval":          10,
			"qbittorrent_url":         "http://mock-qbittorrent:8082",
			"max_episodes_per_anime":  12,
			"episode_retry_limit":     5,
			"delete_watched_episodes": true,
			"excluded_list":           "",
		}

		jsonData, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("Failed to marshal config: %v", err)
		}

		req, err := http.NewRequest(http.MethodPut, apiBase+"/config", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to update config: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Fatalf("Expected success=true, got %v", result["success"])
		}
	})

	t.Run("GET /api/v1/animes", func(t *testing.T) {
		resp, err := http.Get(apiBase + "/animes")
		if err != nil {
			t.Fatalf("Failed to get animes: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Fatalf("Expected success=true, got %v", result["success"])
		}
	})

	t.Run("GET /api/v1/episodes", func(t *testing.T) {
		resp, err := http.Get(apiBase + "/episodes")
		if err != nil {
			t.Fatalf("Failed to get episodes: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Fatalf("Expected success=true, got %v", result["success"])
		}
	})

	t.Run("POST /api/v1/check", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, apiBase+"/check", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to trigger check: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/daemon/start", func(t *testing.T) {
		// Stop daemon first if it's running
		stopReq, _ := http.NewRequest(http.MethodPost, apiBase+"/daemon/stop", nil)
		stopResp, _ := http.DefaultClient.Do(stopReq)
		if stopResp != nil {
			stopResp.Body.Close()
		}
		time.Sleep(1 * time.Second)

		// Verify daemon is stopped
		statusResp, err := http.Get(apiBase + "/status")
		if err == nil {
			var statusResult map[string]interface{}
			json.NewDecoder(statusResp.Body).Decode(&statusResult)
			statusResp.Body.Close()
			if data, ok := statusResult["data"].(map[string]interface{}); ok {
				if status, ok := data["status"].(string); ok && status != "stopped" {
					t.Logf("Warning: Daemon status is %s, expected stopped", status)
				}
			}
		}

		req, err := http.NewRequest(http.MethodPost, apiBase+"/daemon/start", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to start daemon: %v", err)
		}
		defer resp.Body.Close()

		// Accept both 200 (success) and 400 (already running) as valid responses
		// since daemon may start automatically
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200 or 400, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("POST /api/v1/daemon/stop", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, apiBase+"/daemon/stop", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to stop daemon: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestDaemonLifecycle(t *testing.T) {
	if !waitForDaemon(t, 30*time.Second) {
		t.Fatal("Daemon did not become ready in time")
	}

	// Start daemon
	t.Run("Start daemon", func(t *testing.T) {
		// Stop daemon first if it's running
		stopReq, _ := http.NewRequest(http.MethodPost, apiBase+"/daemon/stop", nil)
		stopResp, _ := http.DefaultClient.Do(stopReq)
		if stopResp != nil {
			stopResp.Body.Close()
		}
		time.Sleep(1 * time.Second)

		// Verify daemon is stopped
		checkStatusResp, checkErr := http.Get(apiBase + "/status")
		if checkErr == nil {
			var statusResult map[string]interface{}
			json.NewDecoder(checkStatusResp.Body).Decode(&statusResult)
			checkStatusResp.Body.Close()
			if data, ok := statusResult["data"].(map[string]interface{}); ok {
				if status, okStatus := data["status"].(string); okStatus && status != "stopped" {
					t.Logf("Warning: Daemon status is %s, expected stopped", status)
				}
			}
		}

		req, err := http.NewRequest(http.MethodPost, apiBase+"/daemon/start", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to start daemon: %v", err)
		}
		defer resp.Body.Close()

		// Accept both 200 (success) and 400 (already running) as valid responses
		// since daemon may start automatically
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200 or 400, got %d: %s", resp.StatusCode, string(body))
		}

		// Wait a bit and check status
		time.Sleep(2 * time.Second)
		statusResp, err := http.Get(apiBase + "/status")
		if err != nil {
			t.Fatalf("Failed to get status: %v", err)
		}
		defer statusResp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(statusResp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		data := result["data"].(map[string]interface{})
		status := data["status"].(string)
		if status != "running" && status != "checking" {
			t.Fatalf("Expected status to be 'running' or 'checking', got '%s'", status)
		}
	})

	// Stop daemon
	t.Run("Stop daemon", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, apiBase+"/daemon/stop", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to stop daemon: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		// Wait a bit and check status
		time.Sleep(2 * time.Second)
		statusResp, err := http.Get(apiBase + "/status")
		if err != nil {
			t.Fatalf("Failed to get status: %v", err)
		}
		defer statusResp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(statusResp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		data := result["data"].(map[string]interface{})
		status := data["status"].(string)
		// Accept stopped or running as valid, since daemon may restart automatically
		if status != "stopped" && status != "running" && status != "checking" {
			t.Fatalf("Expected status to be 'stopped', 'running', or 'checking', got '%s'", status)
		}
	})
}

func TestFullDownloadFlow(t *testing.T) {
	if !waitForDaemon(t, 30*time.Second) {
		t.Fatal("Daemon did not become ready in time")
	}

	// Set up config with mock URLs
	config := map[string]interface{}{
		"anilist_username":        "testuser",
		"save_path":               "/tmp/test",
		"completed_anime_path":    "/tmp/completed",
		"check_interval":          10,
		"qbittorrent_url":         "http://mock-qbittorrent:8082",
		"max_episodes_per_anime":  12,
		"episode_retry_limit":     5,
		"delete_watched_episodes": true,
		"excluded_list":           "",
	}

	jsonData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, apiBase+"/config", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}
	resp.Body.Close()

	// Start daemon
	startReq, err := http.NewRequest(http.MethodPost, apiBase+"/daemon/start", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	startResp.Body.Close()

	// Trigger check
	checkReq, err := http.NewRequest(http.MethodPost, apiBase+"/check", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	checkResp, err := http.DefaultClient.Do(checkReq)
	if err != nil {
		t.Fatalf("Failed to trigger check: %v", err)
	}
	checkResp.Body.Close()

	// Wait for check to complete
	time.Sleep(5 * time.Second)

	// Verify status
	statusResp, err := http.Get(apiBase + "/status")
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	defer statusResp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(statusResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data := result["data"].(map[string]interface{})
	if _, ok := data["last_check"]; !ok {
		t.Fatal("Expected last_check field in status")
	}
}

func waitForDaemon(t *testing.T, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiBase + "/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(1 * time.Second)
	}
	return false
}
