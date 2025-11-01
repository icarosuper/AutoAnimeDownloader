package torrents

import (
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
	httpClient HTTPClient
	baseURL    string
	savePath   string
}

func NewTorrentService(httpClient HTTPClient, qBittorrentURL string, savePath string) *TorrentService {
	return &TorrentService{
		httpClient: httpClient,
		baseURL:    getBaseUrl(qBittorrentURL),
		savePath:   savePath,
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

func (ts *TorrentService) DownloadTorrent(magnet string, animeName string, epName string) string {
	err := ts.addTorrent(magnet, ts.savePath, animeName, epName)
	if err != nil {
		fmt.Println("Failed to add torrent for:", epName)
		return ""
	}

	// Tijolo pra esperar o torrent ser adicionado completamente
	// TODO: Remover quando imbutir torrent no projeto
	time.Sleep(50 * time.Millisecond)

	hash := ts.getTorrentsHash(epName)
	if hash == "" {
		fmt.Println("Failed to retrieve torrent hash for:", epName)
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
		fmt.Println("Error deleting torrents:", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

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

func (ts *TorrentService) addTorrent(magnet string, savePath string, animeName string, epName string) error {
	values := url.Values{}

	sanitizedAnimeName := sanitizeFolderName(animeName)
	path := filepath.Join(savePath, sanitizedAnimeName)

	values.Add("urls", magnet)
	values.Add("savepath", path)
	values.Add("category", CATEGORY)
	values.Add("rename", epName)

	resp, err := ts.httpClient.PostForm(ts.baseURL+"/add", values)
	if err != nil {
		fmt.Printf("Error adding %s to torrent: %v\n", epName, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

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

func (ts *TorrentService) getDownloadedTorrents() ([]Torrent, error) {
	response, err := ts.httpClient.Get(ts.baseURL + "/info" + "?category=" + CATEGORY)
	if err != nil {
		fmt.Println("Error fetching torrents:", err)
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	var torrents []Torrent
	err = json.NewDecoder(response.Body).Decode(&torrents)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return nil, err
	}

	return torrents, nil
}

func getBaseUrl(qBittorrentUrl string) string {
	fullUrl := qBittorrentUrl + "/api/v2/torrents"
	return fullUrl
}
