package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type Torrent struct {
	Hash        string `json:"hash"`
	Magnet      string `json:"magnet_uri"`
	Name        string `json:"name"`
	SavePath    string `json:"save_path"`
	ContentPath string `json:"content_path"`
}

var (
	torrents = make(map[string]Torrent)
	mu       sync.RWMutex
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	http.HandleFunc("/api/v2/torrents/info", handleInfo)
	http.HandleFunc("/api/v2/torrents/add", handleAdd)
	http.HandleFunc("/api/v2/torrents/delete", handleDelete)
	http.HandleFunc("/api/v2/torrents/setLocation", handleSetLocation)

	log.Printf("Mock qBittorrent API server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	category := r.URL.Query().Get("category")
	if category != "autoAnimeDownloader" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	mu.RLock()
	result := make([]Torrent, 0, len(torrents))
	for _, torrent := range torrents {
		result = append(result, torrent)
	}
	mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	magnet := r.FormValue("urls")
	savePath := r.FormValue("savepath")
	rename := r.FormValue("rename")

	if magnet == "" {
		http.Error(w, "Missing magnet URL", http.StatusBadRequest)
		return
	}

	// Generate a mock hash
	hash := fmt.Sprintf("mockhash%d", len(torrents)+1)
	name := rename
	if name == "" {
		name = "Mock Torrent"
	}

	torrent := Torrent{
		Hash:        hash,
		Magnet:      magnet,
		Name:        name,
		SavePath:    savePath,
		ContentPath: savePath,
	}

	mu.Lock()
	torrents[hash] = torrent
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	hashesStr := r.FormValue("hashes")
	if hashesStr == "" {
		http.Error(w, "Missing hashes", http.StatusBadRequest)
		return
	}

	hashes := strings.Split(hashesStr, "|")

	mu.Lock()
	for _, hash := range hashes {
		delete(torrents, hash)
	}
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func handleSetLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	hashesStr := r.FormValue("hashes")
	location := r.FormValue("location")

	if hashesStr == "" || location == "" {
		http.Error(w, "Missing hashes or location", http.StatusBadRequest)
		return
	}

	hashes := strings.Split(hashesStr, "|")

	mu.Lock()
	for _, hash := range hashes {
		if torrent, exists := torrents[hash]; exists {
			torrent.SavePath = location
			torrent.ContentPath = location
			torrents[hash] = torrent
		}
	}
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}
