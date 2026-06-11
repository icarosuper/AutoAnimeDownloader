package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// mockQBitHTTPClient implements torrents.HTTPClient and captures DeleteTorrents and add calls.
type mockQBitHTTPClient struct {
	deletedHashes []string
	addCallCount  int
}

func (m *mockQBitHTTPClient) Get(rawURL string) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("[]")),
	}, nil
}

func (m *mockQBitHTTPClient) PostForm(rawURL string, data url.Values) (*http.Response, error) {
	if strings.HasSuffix(rawURL, "/delete") {
		if hashes := data.Get("hashes"); hashes != "" {
			m.deletedHashes = append(m.deletedHashes, strings.Split(hashes, "|")...)
		}
	}
	if strings.HasSuffix(rawURL, "/add") {
		m.addCallCount++
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// mockFileManagerForEpisodes implements FileManagerInterface minimally for episode tests.
type mockFileManagerForEpisodes struct {
	deletedEpisodeIDs []int
}

func (m *mockFileManagerForEpisodes) LoadConfigs() (*files.Config, error)               { return nil, nil }
func (m *mockFileManagerForEpisodes) SaveConfigs(*files.Config) error                   { return nil }
func (m *mockFileManagerForEpisodes) LoadSavedEpisodes() ([]files.EpisodeStruct, error) { return nil, nil }
func (m *mockFileManagerForEpisodes) SaveEpisodesToFile([]files.EpisodeStruct) error    { return nil }
func (m *mockFileManagerForEpisodes) DeleteEpisodesFromFile(ids []int) error {
	m.deletedEpisodeIDs = append(m.deletedEpisodeIDs, ids...)
	return nil
}
func (m *mockFileManagerForEpisodes) DeleteEmptyFolders(string, string) error                   { return nil }
func (m *mockFileManagerForEpisodes) LoadBlockedEpisodes() ([]int, error)                       { return nil, nil }
func (m *mockFileManagerForEpisodes) BlockEpisode(int) error                                    { return nil }
func (m *mockFileManagerForEpisodes) UnblockEpisode(int) error                                  { return nil }
func (m *mockFileManagerForEpisodes) UnmanageEpisode(int) error                                 { return nil }
func (m *mockFileManagerForEpisodes) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) LoadAnimeSettings(int) (*files.AnimeSettings, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) SaveAnimeSettings(int, files.AnimeSettings) error { return nil }

func containsHash(hashes []string, target string) bool {
	for _, h := range hashes {
		if h == target {
			return true
		}
	}
	return false
}

func containsID(ids []int, target int) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// TestEpisodeInTorrents_HashMatch verifica que um episódio salvo com hash presente no
// qBittorrent é considerado "em torrents", mesmo que o nome do torrent não bata.
// Cobre o caso de batch torrents com nome original do grupo (ex: [EMBER]) que não corresponde
// ao título do anime usado como chave.
func TestEpisodeInTorrents_HashMatch(t *testing.T) {
	const savedHash = "601d1465c25e4e47da30d891ebfeea046bfefdee"

	hashSet := map[string]bool{
		"[EMBER] Mairimashita! Iruma-kun (2021) (Season 2) [1080p]": false, // name key, not relevant
		savedHash: true,
	}

	if !episodeInTorrents(savedHash, hashSet) {
		t.Error("episódio deve ser considerado em torrents quando hash bate")
	}
}

// TestEpisodeInTorrents_HashMissing verifica que um episódio cujo torrent foi removido
// do qBittorrent é marcado para re-download.
func TestEpisodeInTorrents_HashMissing(t *testing.T) {
	hashSet := map[string]bool{
		"outrohash": true,
	}

	if episodeInTorrents("601d1465c25e4e47da30d891ebfeea046bfefdee", hashSet) {
		t.Error("episódio não deve ser considerado em torrents quando hash está ausente")
	}
}

// TestEpisodeInTorrents_EmptyHash verifica que episódio sem hash salvo (não baixado)
// nunca é considerado "em torrents", mesmo que a string vazia seja uma chave no mapa.
func TestEpisodeInTorrents_EmptyHash(t *testing.T) {
	hashSet := map[string]bool{
		"": true, // garante que a guarda savedHash != "" é necessária
	}

	if episodeInTorrents("", hashSet) {
		t.Error("hash vazio não deve ser considerado em torrents")
	}
}

// TestDeleteEpisodesByStatus_DroppedAnime verifica que episódios de um anime dropado
// são deletados do qBittorrent e removidos do arquivo de episódios.
func TestDeleteEpisodesByStatus_DroppedAnime(t *testing.T) {
	const animeID = 100
	const episodeID = 42
	const episodeHash = "abc123hash"

	deleteListResponse := &anilist.AniListResponse{}
	deleteListResponse.Data.Page.MediaList = []anilist.MediaList{
		{Id: animeID},
	}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:    episodeID,
			AnimeID:      animeID,
			EpisodeHash:  episodeHash,
			AnimeName:    "Dropped Anime",
			DownloadDate: time.Now(),
		},
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deleteListResponse, fm, ts, savedEpisodes)

	if !containsHash(mockHTTP.deletedHashes, episodeHash) {
		t.Errorf("esperava hash %q deletado do qBittorrent, obteve %v", episodeHash, mockHTTP.deletedHashes)
	}
	if !containsID(fm.deletedEpisodeIDs, episodeID) {
		t.Errorf("esperava episódio ID %d removido do arquivo, obteve %v", episodeID, fm.deletedEpisodeIDs)
	}
}

// TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted verifica que episódios marcados como
// manualmente gerenciados não são deletados mesmo que o anime esteja dropado.
func TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted(t *testing.T) {
	const animeID = 200
	const episodeID = 99
	const episodeHash = "manualhash"

	deleteListResponse := &anilist.AniListResponse{}
	deleteListResponse.Data.Page.MediaList = []anilist.MediaList{
		{Id: animeID},
	}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:       episodeID,
			AnimeID:         animeID,
			EpisodeHash:     episodeHash,
			AnimeName:       "Dropped Anime",
			ManuallyManaged: true,
		},
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deleteListResponse, fm, ts, savedEpisodes)

	if containsHash(mockHTTP.deletedHashes, episodeHash) {
		t.Error("episódio manualmente gerenciado não deve ser deletado do qBittorrent")
	}
	if containsID(fm.deletedEpisodeIDs, episodeID) {
		t.Error("episódio manualmente gerenciado não deve ser removido do arquivo")
	}
}

// TestCheckEpisode_BlacklistedEpisodeMarkedForDeletion verifica que um episódio já baixado
// de um anime na blacklist é marcado para deleção (shouldDelete=true).
func TestCheckEpisode_BlacklistedEpisodeMarkedForDeletion(t *testing.T) {
	englishTitle := "Blacklisted Anime"
	anime := anilist.MediaList{
		Id:       300,
		Progress: 0,
		Media: anilist.Media{
			Title: anilist.Title{English: &englishTitle},
		},
		CustomLists: anilist.CustomLists{"Blacklist": true},
	}

	ep := anilist.AiringNode{ID: 55, Episode: 1, TimeUntilAiring: -100}

	configs := &files.Config{
		ExcludedLists:       []string{"Blacklist"},
		MaxEpisodesPerAnime: 12,
	}

	downloaded := 0
	shouldDownload, shouldDelete := checkEpisode(configs, ep, anime, true, &downloaded, false, false)

	if shouldDownload {
		t.Error("episódio de anime na blacklist não deve ser baixado")
	}
	if !shouldDelete {
		t.Error("episódio já baixado de anime na blacklist deve ser marcado para deleção")
	}
}

// TestHandleSavedEpisodes_BlacklistedAnime_DeletesTorrents verifica que episódios marcados
// para deleção (ex: anime na blacklist) são de fato deletados do qBittorrent.
func TestHandleSavedEpisodes_BlacklistedAnime_DeletesTorrents(t *testing.T) {
	const episodeID = 55
	const episodeHash = "blacklisthash"

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:   episodeID,
			AnimeID:     300,
			EpisodeHash: episodeHash,
			AnimeName:   "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: true,
		MaxEpisodesPerAnime:   12,
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     []int{episodeID},
		checkedEpisodes: []int{episodeID},
	}

	handleSavedEpisodes(fm, configs, ts, data)

	if !containsHash(mockHTTP.deletedHashes, episodeHash) {
		t.Errorf("esperava hash %q deletado do qBittorrent, obteve %v", episodeHash, mockHTTP.deletedHashes)
	}
}

// TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesIdsToDelete verifica que episódios já
// baixados de um anime na blacklist são incluídos em idsToDelete no resultado.
func TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesIdsToDelete(t *testing.T) {
	const episodeID = 77
	const animeID = 300

	englishTitle := "Blacklisted Anime"
	anime := anilist.MediaList{
		Id:       animeID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Status: anilist.MediaStatusReleasing,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: []anilist.AiringNode{
					{ID: episodeID, Episode: 1, TimeUntilAiring: -100},
				},
			},
		},
		CustomLists: anilist.CustomLists{"Blacklist": true},
	}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:   episodeID,
			AnimeID:     animeID,
			EpisodeHash: "somehash",
			AnimeName:   "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		ExcludedLists:       []string{"Blacklist"},
		MaxEpisodesPerAnime: 12,
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")

	result := processAnimeEpisodes(configs, ts, anime, nil, savedEpisodes, map[int]bool{}, "", nil, defaultNyaaSearcher())

	if !containsID(result.idsToDelete, episodeID) {
		t.Errorf("esperava episode ID %d em idsToDelete, obteve %v", episodeID, result.idsToDelete)
	}
	if len(result.newEpisodes) > 0 {
		t.Error("anime na blacklist não deve ter novos episódios baixados")
	}
}

// TestHandleSavedEpisodes_BlacklistedAnime_NoDeleteWhenFlagOff verifica que episódios
// marcados para deleção NÃO são deletados quando DeleteWatchedEpisodes=false.
func TestHandleSavedEpisodes_BlacklistedAnime_NoDeleteWhenFlagOff(t *testing.T) {
	const episodeID = 55
	const episodeHash = "blacklisthash"

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:   episodeID,
			AnimeID:     300,
			EpisodeHash: episodeHash,
			AnimeName:   "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: false,
		MaxEpisodesPerAnime:   12,
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     []int{episodeID},
		checkedEpisodes: []int{episodeID},
	}

	handleSavedEpisodes(fm, configs, ts, data)

	if containsHash(mockHTTP.deletedHashes, episodeHash) {
		t.Error("episódio não deve ser deletado quando DeleteWatchedEpisodes=false")
	}
}

// TestProcessAnimeEpisodes_BatchNoRedownload é um teste de regressão para o bug onde
// animes completos eram re-baixados em loop porque a verificação usava nome em vez de hash.
// O torrent "[EMBER] Mairimashita!..." tem um nome que nunca casa com a chave de busca por nome,
// mas o hash "601d1465..." bate — o código correto deve detectar isso e não acionar re-download.
func TestProcessAnimeEpisodes_BatchNoRedownload(t *testing.T) {
	const batchHash = "601d1465c25e4e47da30d891ebfeea046bfefdee"
	const animeID = 101972
	englishTitle := "Mairimashita! Iruma-kun Season 2"

	// 12 nós de episódio, todos já ao ar
	nodes := make([]anilist.AiringNode, 12)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 1000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}

	anime := anilist.MediaList{
		Id:       animeID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Status: anilist.MediaStatusFinished,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: nodes,
			},
		},
	}

	// Todos os 12 episódios já salvos, todos com o mesmo hash do torrent batch
	savedEpisodes := make([]files.EpisodeStruct, 12)
	for i := range savedEpisodes {
		savedEpisodes[i] = files.EpisodeStruct{
			EpisodeID:   1000 + i,
			AnimeID:     animeID,
			EpisodeHash: batchHash,
			AnimeName:   englishTitle,
			DownloadDate: time.Now(),
		}
	}

	// qBittorrent tem o torrent batch com o nome original do grupo (não bate por nome)
	dlTorrents := []torrents.Torrent{
		{
			Name: "[EMBER] Mairimashita! Iruma-kun (2021) (Season 2) [1080p][HEVC][AAC]",
			Hash: batchHash,
		},
	}

	// Mock do Nyaa: se searchBatch for chamado, o teste deve falhar
	searchBatchCalled := false
	mockSearcher := nyaaSearcher{
		searchBatch: func(_ anilist.Title, _ []string, _ string) []nyaa.TorrentResult {
			searchBatchCalled = true
			return []nyaa.TorrentResult{{MagnetLink: "magnet:?xt=urn:btih:fakehash"}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ string) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool, _ string) []nyaa.TorrentResult {
			return nil
		},
		searchMultiple: func(_ anilist.Title, _ []string, _ []int, _ string) []nyaa.TorrentResult {
			return nil
		},
	}

	configs := &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   1,
	}

	mockHTTP := &mockQBitHTTPClient{}
	ts := torrents.NewTorrentService(mockHTTP, "http://localhost:8080", "/save", "")

	result := processAnimeEpisodes(configs, ts, anime, dlTorrents, savedEpisodes, map[int]bool{}, "", nil, mockSearcher)

	if searchBatchCalled {
		t.Error("searchBatch não deve ser chamado: todos os episódios já estão no qBittorrent pelo hash")
	}
	if mockHTTP.addCallCount > 0 {
		t.Errorf("POST /add não deve ser chamado, mas foi chamado %d vez(es)", mockHTTP.addCallCount)
	}
	if len(result.newEpisodes) > 0 {
		t.Errorf("newEpisodes deve estar vazio, obteve %d episódio(s)", len(result.newEpisodes))
	}
	if len(result.idsToDelete) > 0 {
		t.Errorf("idsToDelete deve estar vazio, obteve %v", result.idsToDelete)
	}
}
