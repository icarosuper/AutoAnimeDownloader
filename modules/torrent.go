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

func DownloadAnime(config Config, magnet string, animeName string, episode int) string {
	baseUrl := getBaseUrl(config.QBittorrentUrl)

	episodeName := addTorrent(baseUrl, magnet, config.SavePath, animeName, episode)

	hash := getTorrentsHash(baseUrl, episodeName)
	if hash == "" {
		fmt.Println("Failed to retrieve torrent hash for:", episodeName)
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

func TestQBittorrentConnection(config Config) bool {
	baseUrl := getBaseUrl(config.QBittorrentUrl)

	response, err := http.Get(baseUrl + "/info")
	if err != nil {
		fmt.Println("Error connecting to qBittorrent:", err)
		return false
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		fmt.Println("qBittorrent returned status code:", response.StatusCode)
		return false
	}

	return true
}

func addTorrent(qBittorrentUrl string, magnet string, savePath string, animeName string, episode int) string {
	values := url.Values{}

	path := filepath.Join(savePath, animeName)
	episodeName := fmt.Sprintf("%s EP %02d", animeName, episode)

	values.Add("urls", magnet)
	values.Add("savepath", path)
	values.Add("category", CATEGORY)
	values.Add("rename", episodeName)

	resp, err := http.PostForm(qBittorrentUrl+"/add", values)
	if err != nil {
		fmt.Println("Error adding torrent:", err)
		return episodeName
	}
	defer func() { _ = resp.Body.Close() }()

	return episodeName
}

func getTorrentsHash(qBittorrentUrl string, torrentName string) string {
	response, err := http.Get(qBittorrentUrl + "/info" + "?category=" + CATEGORY)
	if err != nil {
		fmt.Println("Error fetching torrents:", err)
		return ""
	}
	defer func() { _ = response.Body.Close() }()

	var torrents []Torrent
	err = json.NewDecoder(response.Body).Decode(&torrents)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return ""
	}

	for _, torrent := range torrents {
		if strings.Contains(torrent.Name, torrentName) {
			return torrent.Hash
		}
	}

	return ""
}

func getBaseUrl(qBittorrentUrl string) string {
	fullUrl := qBittorrentUrl + "/api/v2/torrents"
	return fullUrl

	//parsedUrl, err := url.Parse(fullUrl)
	//if err != nil {
	//	return fullUrl
	//}
	//return fmt.Sprintf("%s://%s", parsedUrl.Scheme, parsedUrl.Host)
}
