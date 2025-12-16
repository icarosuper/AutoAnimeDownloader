package torrents

import (
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type HTTPClient interface {
	Get(url string) (*http.Response, error)
	PostForm(url string, data url.Values) (*http.Response, error)
}

type DefaultHTTPClient struct{}

func (c *DefaultHTTPClient) Get(url string) (*http.Response, error) {
	return http.Get(url)
}
func (c *DefaultHTTPClient) PostForm(url string, data url.Values) (*http.Response, error) {
	return http.PostForm(url, data)
}

type TorrentService struct {
	httpClient    HTTPClient
	baseURL       string
	savePath      string
	completedPath string
}

func NewTorrentService(httpClient HTTPClient, qBittorrentURL string, savePath string, completedPath string) *TorrentService {
	return &TorrentService{
		httpClient:    httpClient,
		baseURL:       getBaseUrl(qBittorrentURL),
		savePath:      savePath,
		completedPath: completedPath,
	}
}

type Torrent struct {
	Hash        string `json:"hash"`
	Magnet      string `json:"magnet_uri"`
	Name        string `json:"name"`
	SavePath    string `json:"save_path"`
	ContentPath string `json:"content_path"`
}

const CATEGORY = "autoAnimeDownloader"

func (ts *TorrentService) DownloadTorrent(magnet string, animeName string, epName string, animeIsCompleted bool) string {
	err := ts.addTorrent(magnet, animeName, animeIsCompleted, epName)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Str("episode", epName).
			Str("anime_name", animeName).
			Msg("Failed to add torrent")
		return ""
	}

	// Tijolo pra esperar o torrent ser adicionado completamente
	// TODO: Remover quando imbutir torrent no projeto
	time.Sleep(50 * time.Millisecond)

	hash := ts.getTorrentsHash(epName)
	if hash == "" {
		logger.Logger.Warn().
			Str("episode", epName).
			Str("anime_name", animeName).
			Msg("Failed to retrieve torrent hash")
		return ""
	}

	return hash
}

func (ts *TorrentService) DeleteTorrents(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}

	values := url.Values{}
	values.Add("hashes", strings.Join(hashes, "|"))
	values.Add("deleteFiles", "true")

	resp, err := ts.httpClient.PostForm(ts.baseURL+"/delete", values)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Int("count", len(hashes)).
			Msg("Failed to delete torrents via qBittorrent API")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 512)
		resp.Body.Read(bodyBytes)
		return fmt.Errorf("qBittorrent API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (ts *TorrentService) SendAnimeToCompletedFolder(hashes []string, animeName string) error {
	newSavePath := ts.getFolderName(animeName, true)

	values := url.Values{}
	values.Add("hashes", strings.Join(hashes, "|"))
	values.Add("location", newSavePath)

	resp, err := ts.httpClient.PostForm(ts.baseURL+"/setLocation", values)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Str("anime_name", animeName).
			Int("count", len(hashes)).
			Str("destination", newSavePath).
			Msg("Failed to set save path for torrents")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 512)
		resp.Body.Read(bodyBytes)
		return fmt.Errorf("qBittorrent API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (ts *TorrentService) GetDownloadedTorrents() ([]Torrent, error) {
	return ts.getDownloadedTorrents()
}

func sanitizeFolderName(name string) string {
	invalidChars := []string{":", "<", ">", "|", "?", "*", "\"", "\\", "/"}

	sanitized := name
	for _, char := range invalidChars {
		sanitized = strings.ReplaceAll(sanitized, char, "")
	}

	// Remover informações de temporada/cour do nome da pasta
	seasonPattern := regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)
	sanitized = seasonPattern.ReplaceAllString(sanitized, "")

	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.ReplaceAll(sanitized, "  ", " ")

	return sanitized
}

func removeSpecialCharacters(s string) string {
	// Converte para minúsculas
	s = strings.ToLower(s)
	// Remove tudo exceto letras, números e espaços
	re := regexp.MustCompile(`[^a-z0-9\s]`)
	s = re.ReplaceAllString(s, "")
	// Remove espaços múltiplos e trim
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	return s
}

func (ts *TorrentService) addTorrent(magnet string, animeName string, animeIsCompleted bool, epName string) error {
	values := url.Values{}

	path := ts.getFolderName(animeName, animeIsCompleted)

	logger.Logger.Debug().
		Str("episode", epName).
		Str("anime_name", animeName).
		Str("savepath", path).
		Bool("anime_completed", animeIsCompleted).
		Msg("Adding torrent to qBittorrent")

	values.Add("urls", magnet)
	values.Add("savepath", path)
	values.Add("category", CATEGORY)
	values.Add("rename", epName)

	resp, err := ts.httpClient.PostForm(ts.baseURL+"/add", values)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Str("episode", epName).
			Str("savepath", path).
			Msg("Error adding torrent to qBittorrent")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Verificar status code da resposta
	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 512)
		resp.Body.Read(bodyBytes)
		err := fmt.Errorf("qBittorrent API returned status %d: %s", resp.StatusCode, string(bodyBytes))
		logger.Logger.Error().
			Err(err).
			Str("episode", epName).
			Str("savepath", path).
			Int("status_code", resp.StatusCode).
			Msg("qBittorrent API returned error")
		return err
	}

	logger.Logger.Debug().
		Str("episode", epName).
		Str("savepath", path).
		Msg("Successfully sent savepath to qBittorrent")

	return nil
}

func (ts *TorrentService) getTorrentsHash(torrentName string) string {
	torrents, err := ts.getDownloadedTorrents()
	if err != nil {
		return ""
	}

	for _, torrent := range torrents {
		// Tenta match exato primeiro (mais rápido)
		if strings.Contains(torrent.Name, torrentName) {
			return torrent.Hash
		}
		// Se não encontrar, tenta removendo caracteres especiais
		cleanName := removeSpecialCharacters(torrent.Name)
		cleanTorrentName := removeSpecialCharacters(torrentName)
		if cleanTorrentName != "" && strings.Contains(cleanName, cleanTorrentName) {
			return torrent.Hash
		}
	}

	return ""
}

func (ts *TorrentService) getFolderName(animeName string, animeIsCompleted bool) string {
	savePath := ts.savePath
	if animeIsCompleted && ts.completedPath != "" && ts.completedPath != ts.savePath {
		savePath = ts.completedPath
	}

	sanitizedAnimeName := sanitizeFolderName(animeName)
	return filepath.Join(savePath, sanitizedAnimeName)
}

func (ts *TorrentService) getDownloadedTorrents() ([]Torrent, error) {
	response, err := ts.httpClient.Get(ts.baseURL + "/info" + "?category=" + CATEGORY)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Msg("Failed to fetch downloaded torrents from qBittorrent API")
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		n, _ := response.Body.Read(bodyBytes)
		responseBody := strings.TrimSpace(string(bodyBytes[:n]))
		if responseBody == "" {
			responseBody = "(empty response)"
		}
		return nil, fmt.Errorf("qBittorrent API returned status %d: %s", response.StatusCode, responseBody)
	}

	// Check Content-Type to ensure we're getting JSON
	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		bodyBytes := make([]byte, 1024)
		n, _ := response.Body.Read(bodyBytes)
		responseBody := strings.TrimSpace(string(bodyBytes[:n]))
		if responseBody == "" {
			responseBody = "(empty response)"
		}
		return nil, fmt.Errorf("qBittorrent API returned non-JSON response (Content-Type: %s): %s", contentType, responseBody)
	}

	var torrents []Torrent
	err = json.NewDecoder(response.Body).Decode(&torrents)
	if err != nil {
		logger.Logger.Error().
			Err(err).
			Msg("Failed to decode qBittorrent torrents response")
		return nil, fmt.Errorf("failed to decode qBittorrent response: %w", err)
	}

	return torrents, nil
}

func getBaseUrl(qBittorrentUrl string) string {
	fullUrl := qBittorrentUrl + "/api/v2/torrents"
	return fullUrl
}
