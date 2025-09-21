package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type Torrent struct {
	Hash        string `json:"hash"`
	Magnet      string `json:"magnet_uri"`
	Name        string `json:"name"`
	SavePath    string `json:"save_path"`
	ContentPath string `json:"content_path"`
}

const CATEGORY = "autoAnimeDownloader"

func DownloadTorrent(config Config, magnet string, animeName string, epName string) string {
	baseUrl := getBaseUrl(config.QBittorrentUrl)

	err := addTorrent(baseUrl, magnet, config.SavePath, animeName, epName)
	if err != nil {
		fmt.Println("Failed to add torrent for:", epName)
		return ""
	}

	hash := getTorrentsHash(baseUrl, epName)
	if hash == "" {
		fmt.Println("Failed to retrieve torrent hash for:", epName)
		return ""
	}

	return hash
}

func DeleteTorrents(config Config, hashes []string) {
	if len(hashes) == 0 {
		return
	}

	baseUrl := getBaseUrl(config.QBittorrentUrl)
	values := url.Values{}

	values.Add("hashes", strings.Join(hashes, "|"))
	values.Add("deleteFiles", "true")

	resp, err := http.PostForm(baseUrl+"/delete", values)
	if err != nil {
		fmt.Println("Error deleting torrents:", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
}

func GetDownloadedTorrents(config Config) ([]Torrent, error) {
	baseUrl := getBaseUrl(config.QBittorrentUrl)

	return getDownloadedTorrents(baseUrl)
}

func addTorrent(qBittorrentUrl string, magnet string, savePath string, animeName string, epName string) error {
	values := url.Values{}

	path := filepath.Join(savePath, animeName)

	values.Add("urls", magnet)
	values.Add("savepath", path)
	values.Add("category", CATEGORY)
	values.Add("rename", epName)

	resp, err := http.PostForm(qBittorrentUrl+"/add", values)
	if err != nil {
		fmt.Printf("Error adding %s to torrent: %v\n", epName, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

func getTorrentsHash(qBittorrentUrl string, torrentName string) string {
	torrents, err := getDownloadedTorrents(qBittorrentUrl)
	if err != nil {
		return ""
	}

	for _, torrent := range torrents {
		if strings.Contains(torrent.Name, torrentName) {
			return torrent.Hash
		}
	}

	return ""
}

func getDownloadedTorrents(qBittorrentUrl string) ([]Torrent, error) {
	response, err := http.Get(qBittorrentUrl + "/info" + "?category=" + CATEGORY)
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
