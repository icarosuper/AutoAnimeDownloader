package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// testLibrarian returns a real Librarian; with empty LibraryPaths it performs no FS ops.
func testLibrarian() files.Librarian {
	return files.NewLibrarian(files.NewOSFileSystem())
}

// fakeWithTorrents returns a FakeBackend pre-populated with completed torrents by hash.
func fakeWithTorrents(hashes ...string) *torrents.FakeBackend {
	fb := torrents.NewFakeBackend()
	for _, h := range hashes {
		fb.AddCompleted(h, "/data/"+h)
	}
	return fb
}

// mockFileManagerForEpisodes implements FileManagerInterface minimally for episode tests.
type mockFileManagerForEpisodes struct {
	deletedEpisodeKeys []files.EpisodeKey
	blockedEpisodeKeys []files.EpisodeKey
	blockErr           error
	standaloneAnimes   []int
	removedStandalone  []int
	savedEpisodes      []files.EpisodeStruct
	settings           map[int]files.AnimeSettings
}

func (m *mockFileManagerForEpisodes) LoadConfigs() (*files.Config, error) { return nil, nil }
func (m *mockFileManagerForEpisodes) SaveConfigs(*files.Config) error     { return nil }
func (m *mockFileManagerForEpisodes) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	return m.savedEpisodes, nil
}
func (m *mockFileManagerForEpisodes) SaveEpisodesToFile([]files.EpisodeStruct) error { return nil }
func (m *mockFileManagerForEpisodes) UpsertEpisodes([]files.EpisodeStruct) error     { return nil }
func (m *mockFileManagerForEpisodes) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	m.deletedEpisodeKeys = append(m.deletedEpisodeKeys, keys...)
	return nil
}
func (m *mockFileManagerForEpisodes) DeleteEmptyFolders(string) error { return nil }
func (m *mockFileManagerForEpisodes) LoadBlockedEpisodes() ([]files.EpisodeKey, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) BlockEpisode(key files.EpisodeKey) error {
	m.blockedEpisodeKeys = append(m.blockedEpisodeKeys, key)
	return m.blockErr
}
func (m *mockFileManagerForEpisodes) UnblockEpisode(files.EpisodeKey) error  { return nil }
func (m *mockFileManagerForEpisodes) UnmanageEpisode(files.EpisodeKey) error { return nil }
func (m *mockFileManagerForEpisodes) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) LoadAnimeSettings(id int) (*files.AnimeSettings, error) {
	if s, ok := m.settings[id]; ok {
		return &s, nil
	}
	return nil, nil
}
func (m *mockFileManagerForEpisodes) SaveAnimeSettings(int, files.AnimeSettings) error { return nil }
func (m *mockFileManagerForEpisodes) LoadStandaloneAnimes() ([]int, error) {
	return m.standaloneAnimes, nil
}
func (m *mockFileManagerForEpisodes) AddStandaloneAnime(id int) error {
	m.standaloneAnimes = append(m.standaloneAnimes, id)
	return nil
}
func (m *mockFileManagerForEpisodes) RemoveStandaloneAnime(id int) error {
	m.removedStandalone = append(m.removedStandalone, id)
	var kept []int
	for _, existing := range m.standaloneAnimes {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	m.standaloneAnimes = kept
	return nil
}

func containsHash(hashes []string, target string) bool {
	for _, h := range hashes {
		if h == target {
			return true
		}
	}
	return false
}

func containsID[T comparable](items []T, target T) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// epKey monta a chave de episodio que o codigo de producao usa, para os testes nao repetirem o
// literal em cada assercao.
func epKey(animeID, episode int) files.EpisodeKey {
	return files.EpisodeKey{AnimeID: animeID, Episode: episode}
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

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeID,
			AnimeID:       animeID,
			EpisodeHash:   episodeHash,
			AnimeName:     "Dropped Anime",
			DownloadDate:  time.Now(),
		},
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("esperava torrent %q removido do cliente, mas ainda presente", episodeHash)
	}
	if !containsID(fm.deletedEpisodeKeys, epKey(animeID, episodeID)) {
		t.Errorf("esperava episódio ID %d removido do arquivo, obteve %v", episodeID, fm.deletedEpisodeKeys)
	}
}

// TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted verifica que episódios marcados como
// manualmente gerenciados não são deletados mesmo que o anime esteja dropado.
func TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted(t *testing.T) {
	const animeID = 200
	const episodeID = 99
	const episodeHash = "manualhash"

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber:   episodeID,
			AnimeID:         animeID,
			EpisodeHash:     episodeHash,
			AnimeName:       "Dropped Anime",
			ManuallyManaged: true,
		},
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)

	if _, ok := backend.Get(episodeHash); !ok {
		t.Error("episódio manualmente gerenciado não deve ter o torrent removido")
	}
	if containsID(fm.deletedEpisodeKeys, epKey(animeID, episodeID)) {
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
	shouldDownload, shouldDelete, _ := checkEpisode(configs, configs.MaxEpisodesPerAnime, ep, anime, true, &downloaded, false, false, false)

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
			EpisodeNumber: episodeID,
			AnimeID:       300,
			EpisodeHash:   episodeHash,
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: true,
		MaxEpisodesPerAnime:   12,
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    []files.EpisodeKey{epKey(300, episodeID)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, episodeID)},
	}

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), data)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("esperava torrent %q removido do cliente, mas ainda presente", episodeHash)
	}
}

// TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesKeysToDelete verifica que episódios já
// baixados de um anime na blacklist são incluídos em keysToDelete no resultado.
func TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesKeysToDelete(t *testing.T) {
	const episodeNumber = 1
	const animeID = 300

	englishTitle := "Blacklisted Anime"
	anime := anilist.MediaList{
		Id:       animeID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:     animeID,
			Status: anilist.MediaStatusReleasing,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: []anilist.AiringNode{
					{Episode: episodeNumber, TimeUntilAiring: -100},
				},
			},
		},
		CustomLists: anilist.CustomLists{"Blacklist": true},
	}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeNumber,
			AnimeID:       animeID,
			EpisodeHash:   "somehash",
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		ExcludedLists:       []string{"Blacklist"},
		MaxEpisodesPerAnime: 12,
	}

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, nil, savedEpisodes, map[files.EpisodeKey]bool{}, "", defaultNyaaSearcher())

	if !containsID(result.keysToDelete, epKey(animeID, episodeNumber)) {
		t.Errorf("esperava episódio %d em keysToDelete, obteve %v", episodeNumber, result.keysToDelete)
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
			EpisodeNumber: episodeID,
			AnimeID:       300,
			EpisodeHash:   episodeHash,
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: false,
		MaxEpisodesPerAnime:   12,
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    []files.EpisodeKey{epKey(300, episodeID)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, episodeID)},
	}

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), data)

	if _, ok := backend.Get(episodeHash); !ok {
		t.Error("torrent não deve ser removido quando DeleteWatchedEpisodes=false")
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
			Id:     animeID,
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
			EpisodeNumber: i + 1, // deve casar com Episode do node (i+1); numero != Episode dos nodes fazia
			// alreadySaved dar sempre falso e mascarava o teste (issue pre-existente, fora desta task)
			AnimeID:      animeID,
			EpisodeHash:  batchHash,
			AnimeName:    englishTitle,
			DownloadDate: time.Now(),
		}
	}

	// O cliente tem o torrent batch com o nome original do grupo (não bate por nome)
	dlTorrents := []torrents.TorrentInfo{
		{
			Name: "[EMBER] Mairimashita! Iruma-kun (2021) (Season 2) [1080p][HEVC][AAC]",
			Hash: batchHash,
		},
	}

	// Mock do Nyaa: se a busca por anime for chamada, o teste deve falhar
	searchAnimeCalled := false
	mockSearcher := nyaaSearcher{
		searchAnime: func(_ anilist.Title, _ []string, _ []int, _ string) []nyaa.TorrentResult {
			searchAnimeCalled = true
			return []nyaa.TorrentResult{{MagnetLink: "magnet:?xt=urn:btih:fakehash", IsBatch: true}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ string, _ int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool, _ string) []nyaa.TorrentResult {
			return nil
		},
	}

	configs := &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   1,
	}

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, dlTorrents, savedEpisodes, map[files.EpisodeKey]bool{}, "", mockSearcher)

	if searchAnimeCalled {
		t.Error("a busca por anime não deve ser chamada: todos os episódios já estão no cliente pelo hash")
	}
	if len(backend.List()) > 0 {
		t.Errorf("Add não deve ser chamado, mas o cliente tem %d torrent(s)", len(backend.List()))
	}
	if len(result.newEpisodes) > 0 {
		t.Errorf("newEpisodes deve estar vazio, obteve %d episódio(s)", len(result.newEpisodes))
	}
	if len(result.keysToDelete) > 0 {
		t.Errorf("keysToDelete deve estar vazio, obteve %v", result.keysToDelete)
	}
}

// TestDedupeAnimesByMedia verifica que o mesmo anime linkado em contas diferentes é
// colapsado numa única entrada, mantendo a de MENOR progresso (mais conservadora para
// não deletar episódios que alguma conta ainda não assistiu).
func TestDedupeAnimesByMedia(t *testing.T) {
	list := []anilist.MediaList{
		{Id: 1, Progress: 10, Media: anilist.Media{Id: 500}}, // conta A, à frente
		{Id: 2, Progress: 3, Media: anilist.Media{Id: 500}},  // conta B, atrás (mesmo anime)
		{Id: 3, Progress: 7, Media: anilist.Media{Id: 999}},  // outro anime, só conta A
	}

	got := anilist.DedupeByMedia(list)

	if len(got) != 2 {
		t.Fatalf("esperava 2 animes após dedup, obteve %d", len(got))
	}

	var media500 *anilist.MediaList
	for i := range got {
		if got[i].Media.Id == 500 {
			media500 = &got[i]
		}
	}
	if media500 == nil {
		t.Fatal("anime media 500 sumiu do resultado")
	}
	if media500.Progress != 3 {
		t.Errorf("esperava progresso 3 (menor entre contas), obteve %d", media500.Progress)
	}
	if media500.Id != 2 {
		t.Errorf("esperava manter entrada da conta B (Id 2), obteve Id %d", media500.Id)
	}
}

// TestDedupeAnimesByMedia_NoMediaID garante que entradas sem media id (caso inesperado)
// são preservadas em vez de colapsadas todas no id 0.
func TestDedupeAnimesByMedia_NoMediaID(t *testing.T) {
	list := []anilist.MediaList{
		{Id: 1, Progress: 5},
		{Id: 2, Progress: 8},
	}

	got := anilist.DedupeByMedia(list)

	if len(got) != 2 {
		t.Errorf("entradas sem media id não devem ser colapsadas, esperava 2, obteve %d", len(got))
	}
}

// --- saveEpisodesToFile: selective merge on re-download (P1.1 / P1.2) ---

const (
	oldHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// tempFileManager returns a real files.FileManager over a temp dir, so the merge tests
// exercise the actual JSONL round-trip instead of a mock's in-memory slice.
func tempFileManager(t *testing.T) *files.FileManager {
	t.Helper()
	dir := t.TempDir()
	return files.NewManager(
		files.NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "downloaded_episodes"),
		filepath.Join(dir, "blocked_episodes"),
		filepath.Join(dir, "anime_settings"),
		filepath.Join(dir, "standalone_animes"),
	)
}

// loadEpisodeByID reads the persisted episodes back and returns the one with the given ID.
func loadEpisodeByID(t *testing.T, fm FileManagerInterface, id int) files.EpisodeStruct {
	t.Helper()
	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	for _, ep := range all {
		if ep.EpisodeNumber == id {
			return ep
		}
	}
	t.Fatalf("episode %d not found in saved episodes %+v", id, all)
	return files.EpisodeStruct{}
}

// TestSaveEpisodesToFile_RedownloadUpdatesRecord: um episódio já salvo que é re-baixado deve
// terminar com o EpisodeHash NOVO no arquivo — SaveEpisodesToFile é append-only e descartaria a
// atualização, deixando o registro com o hash velho, que é o que o JobOrganize usa para achar o
// torrent (sem isso o hardlink nunca sai).
func TestSaveEpisodesToFile_RedownloadUpdatesRecord(t *testing.T) {
	fm := tempFileManager(t)
	old := time.Now().Add(-48 * time.Hour)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		AnimeName:     "My Anime",
		EpisodeHash:   oldHash,
		EpisodeName:   "My Anime - Episode 5",
		DownloadDate:  old,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		AnimeID:            7,
		AnimeTotalEpisodes: 12,
		AnimeName:          "My Anime",
		EpisodeHash:        newHash,
		EpisodeName:        "My Anime - Episode 5",
		EpisodeNumber:      5,
		IsBatch:            true,
		DownloadDate:       time.Now(),
	}})

	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("esperava 1 registro (merge, não duplicata), obteve %d: %+v", len(all), all)
	}

	got := all[0]
	if got.EpisodeNumber != 5 {
		t.Errorf("EpisodeNumber: esperava 5, obteve %d", got.EpisodeNumber)
	}
	if got.EpisodeHash != newHash {
		t.Errorf("EpisodeHash: esperava %s, obteve %s", newHash, got.EpisodeHash)
	}
	if !got.IsBatch {
		t.Error("IsBatch deve ser atualizado para true")
	}
	if got.AnimeTotalEpisodes != 12 {
		t.Errorf("AnimeTotalEpisodes: esperava 12, obteve %d", got.AnimeTotalEpisodes)
	}
	if !got.DownloadDate.After(old) {
		t.Errorf("DownloadDate deve ser atualizada no re-download, obteve %v", got.DownloadDate)
	}
}

// TestSaveEpisodesToFile_PreservesManuallyManaged garante que o merge não apaga a flag do
// usuário — senão o loop automático passaria a poder deletar um episódio gerenciado à mão.
func TestSaveEpisodesToFile_PreservesManuallyManaged(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber:   5,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		ManuallyManaged: true,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// O loop automático nunca marca ManuallyManaged.
	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	got := loadEpisodeByID(t, fm, 5)
	if !got.ManuallyManaged {
		t.Error("ManuallyManaged deve sobreviver a um re-download automático")
	}
	if got.EpisodeHash != newHash {
		t.Errorf("EpisodeHash deve ser atualizado mesmo preservando ManuallyManaged, obteve %s", got.EpisodeHash)
	}
}

// TestSaveEpisodesToFile_ResetsLibraryPaths garante que os caminhos da biblioteca do release
// ANTIGO são zerados: LibraryPaths vazio é o marcador de "ainda não organizado" que faz o
// JobOrganize criar o hardlink novo e disparar o webhook.
func TestSaveEpisodesToFile_ResetsLibraryPaths(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   oldHash,
		LibraryPaths:  []string{"/library/My Anime/My Anime - E05.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	got := loadEpisodeByID(t, fm, 5)
	if len(got.LibraryPaths) != 0 {
		t.Errorf("LibraryPaths deve ser zerado no re-download, obteve %v", got.LibraryPaths)
	}
}

// TestSaveEpisodesToFile_AppendsNewEpisode garante que o merge não regride o comportamento
// atual: episódio genuinamente novo continua sendo apendado sem tocar nos existentes.
func TestSaveEpisodesToFile_AppendsNewEpisode(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber:   1,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		ManuallyManaged: true,
		LibraryPaths:    []string{"/library/My Anime/My Anime - E01.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 2,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("esperava 2 registros, obteve %d: %+v", len(all), all)
	}

	untouched := loadEpisodeByID(t, fm, 1)
	if !untouched.ManuallyManaged || len(untouched.LibraryPaths) != 1 || untouched.EpisodeHash != oldHash {
		t.Errorf("registro não relacionado foi alterado: %+v", untouched)
	}
	added := loadEpisodeByID(t, fm, 2)
	if added.EpisodeHash != newHash || added.EpisodeNumber != 2 {
		t.Errorf("episódio novo não foi apendado corretamente: %+v", added)
	}
}

// --- error propagation on removal (P1.3) ---

// failingFM is a FileManager mock whose load/delete can be made to fail.
type failingFM struct {
	mockFileManagerForEpisodes
	saved     []files.EpisodeStruct
	loadErr   error
	deleteErr error
	deleted   []files.EpisodeKey
}

func (m *failingFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.saved, nil
}

func (m *failingFM) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	m.deleted = append(m.deleted, keys...)
	return m.deleteErr
}

// TestRemoveEpisodesWithLinks_PropagatesLoadError: falha ao ler o arquivo de episódios deve
// virar erro para o chamador (a API responde 500) em vez de "sucesso" silencioso.
func TestRemoveEpisodesWithLinks_PropagatesLoadError(t *testing.T) {
	fm := &failingFM{loadErr: errors.New("disk on fire")}
	err := RemoveEpisodesWithLinks(fm, torrents.NewFakeBackend(), testLibrarian(), []files.EpisodeKey{{Episode: 1}})
	if err == nil {
		t.Fatal("esperava erro quando LoadSavedEpisodes falha")
	}
}

// TestRemoveEpisodesWithLinks_PropagatesDeleteError: falha ao remover do arquivo deve virar erro.
func TestRemoveEpisodesWithLinks_PropagatesDeleteError(t *testing.T) {
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	fm := &failingFM{
		saved:     []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}},
		deleteErr: errors.New("write failed"),
	}
	backend := fakeWithTorrents(hash)

	err := RemoveEpisodesWithLinks(fm, backend, testLibrarian(), []files.EpisodeKey{{Episode: 1}})
	if err == nil {
		t.Fatal("esperava erro quando DeleteEpisodesFromFile falha")
	}
	// Liberar espaço é best-effort e acontece antes: o torrent sai mesmo assim.
	if _, ok := backend.Get(hash); ok {
		t.Error("torrent deve ser removido mesmo quando a escrita do arquivo falha")
	}
}

// TestRemoveEpisodesWithLinks_SuccessReturnsNil garante que o caminho feliz continua sem erro.
func TestRemoveEpisodesWithLinks_SuccessReturnsNil(t *testing.T) {
	const hash = "dddddddddddddddddddddddddddddddddddddddd"
	fm := &failingFM{saved: []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}}}
	if err := RemoveEpisodesWithLinks(fm, fakeWithTorrents(hash), testLibrarian(), []files.EpisodeKey{epKey(0, 1)}); err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}
	if !containsID(fm.deleted, files.EpisodeKey{Episode: 1}) {
		t.Errorf("esperava episódio 1 deletado do arquivo, obteve %v", fm.deleted)
	}
}

// TestHandleSavedEpisodes_DeleteErrorIsTolerated: o loop automático continua best-effort —
// uma falha ao deletar do arquivo é logada, não aborta o passe nem impede a liberação de espaço.
func TestHandleSavedEpisodes_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	saved := []files.EpisodeStruct{{EpisodeNumber: 55, AnimeID: 300, EpisodeHash: hash}}
	fm := &failingFM{saved: saved, deleteErr: errors.New("write failed")}
	backend := fakeWithTorrents(hash)

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true, MaxEpisodesPerAnime: 12}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		keysToDelete:    []files.EpisodeKey{epKey(300, 55)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, 55)},
	})

	if _, ok := backend.Get(hash); ok {
		t.Error("o passe deve continuar e remover o torrent mesmo com falha na escrita do arquivo")
	}
	if !containsID(fm.deleted, epKey(300, 55)) {
		t.Errorf("a deleção deve ter sido tentada, obteve %v", fm.deleted)
	}
}

// TestDeleteEpisodesByStatus_DeleteErrorIsTolerated: idem para a deleção por status.
func TestDeleteEpisodesByStatus_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "ffffffffffffffffffffffffffffffffffffffff"
	deletableMedia := map[int]bool{100: true}

	saved := []files.EpisodeStruct{{EpisodeNumber: 42, AnimeID: 100, EpisodeHash: hash}}
	fm := &failingFM{saved: saved, deleteErr: errors.New("write failed")}
	backend := fakeWithTorrents(hash)

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), saved)

	if _, ok := backend.Get(hash); ok {
		t.Error("torrent deve ser removido mesmo com falha na escrita do arquivo")
	}
}

// --- RemoveTorrentWithEpisodes (Task 1.3 / 1.4) ---

// countingBackend wraps FakeBackend to count Remove calls per hash — FakeBackend.RemovedKeepData
// alone can't distinguish "called once" from "called N times with the same keepData value".
type countingBackend struct {
	*torrents.FakeBackend
	removeCalls map[string]int
}

func newCountingBackend(hashes ...string) *countingBackend {
	return &countingBackend{FakeBackend: fakeWithTorrents(hashes...), removeCalls: make(map[string]int)}
}

func (c *countingBackend) Remove(hash string, keepData bool) error {
	c.removeCalls[hash]++
	return c.FakeBackend.Remove(hash, keepData)
}

// TestRemoveTorrentWithEpisodes_BatchRemovesAllAndCallsBackendOnce verifica que um batch com N
// episódios do mesmo hash é removido como uma unidade: os N registros saem de uma vez e
// backend.Remove é chamado exatamente uma vez (não uma vez por episódio).
func TestRemoveTorrentWithEpisodes_BatchRemovesAllAndCallsBackendOnce(t *testing.T) {
	const hash = "1111111111111111111111111111111111111111"
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: hash},
		{EpisodeNumber: 2, EpisodeHash: hash},
		{EpisodeNumber: 3, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := newCountingBackend(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}

	for _, ep := range []int{1, 2, 3} {
		if !containsID(fm.deleted, files.EpisodeKey{Episode: ep}) {
			t.Errorf("esperava episódio %d removido do arquivo, obteve %v", ep, fm.deleted)
		}
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("esperava torrent removido do cliente")
	}
	if got := backend.removeCalls[hash]; got != 1 {
		t.Fatalf("esperava backend.Remove chamado exatamente 1 vez para o batch, obteve %d", got)
	}
	if kd, ok := backend.RemovedKeepData[hash]; !ok || kd {
		t.Errorf("esperava backend.Remove chamado com keepData=false para %s, obteve %v (ok=%v)", hash, kd, ok)
	}
}

// TestRemoveTorrentWithEpisodes_KeepDataSkipsLibraryRemoval verifica que keepData=true não
// chama librarian.RemoveFromLibrary e que o valor chega ao backend como keepData=true
// (via FakeBackend.RemovedKeepData).
func TestRemoveTorrentWithEpisodes_KeepDataSkipsLibraryRemoval(t *testing.T) {
	const hash = "2222222222222222222222222222222222222222"
	tmp := t.TempDir()
	libPath := filepath.Join(tmp, "library", "ep.mkv")

	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{libPath}},
	}
	fm := &failingFM{saved: saved}
	backend := fakeWithTorrents(hash)

	// spyLibrarian records whether RemoveFromLibrary was called.
	spy := &spyLibrarian{}

	err := RemoveTorrentWithEpisodes(fm, backend, spy, hash, RemoveTorrentOptions{KeepData: true})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}

	if spy.called {
		t.Error("keepData=true não deve chamar librarian.RemoveFromLibrary")
	}
	if kd, ok := backend.RemovedKeepData[hash]; !ok || !kd {
		t.Errorf("esperava backend.Remove chamado com keepData=true, obteve %v (ok=%v)", kd, ok)
	}
}

// spyLibrarian is a minimal files.Librarian that only records whether RemoveFromLibrary was
// invoked, so tests can assert it was skipped without touching a real filesystem.
type spyLibrarian struct {
	called bool
}

func (s *spyLibrarian) Organize(files.OrganizeRequest) ([]string, error) { return nil, nil }
func (s *spyLibrarian) RemoveFromLibrary(string) error {
	s.called = true
	return nil
}
func (s *spyLibrarian) ProbePath(string) error { return nil }

// TestRemoveTorrentWithEpisodes_OrphanTorrentCallsBackendOnly verifica o caso de torrent órfão
// (nenhum episódio salvo casa com o hash): backend.Remove é chamado, nada é bloqueado, sem erro.
func TestRemoveTorrentWithEpisodes_OrphanTorrentCallsBackendOnly(t *testing.T) {
	const hash = "3333333333333333333333333333333333333333"
	fm := &failingFM{saved: nil}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{Block: true})
	if err != nil {
		t.Fatalf("esperava sucesso para torrent órfão, obteve %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("esperava torrent órfão removido do cliente")
	}
	if len(fm.blockedEpisodeKeys) != 0 {
		t.Errorf("torrent órfão não tem episódio para bloquear, obteve %v", fm.blockedEpisodeKeys)
	}
	if len(fm.deleted) != 0 {
		t.Errorf("torrent órfão não deve tentar deletar registros, obteve %v", fm.deleted)
	}
}

// TestRemoveTorrentWithEpisodes_BlockBlocksAllIDsInGroup verifica que Block=true bloqueia
// todos os ids do grupo antes de remover os registros.
func TestRemoveTorrentWithEpisodes_BlockBlocksAllIDsInGroup(t *testing.T) {
	const hash = "4444444444444444444444444444444444444444"
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 10, EpisodeHash: hash},
		{EpisodeNumber: 20, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{Block: true})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}
	for _, ep := range []int{10, 20} {
		if !containsID(fm.blockedEpisodeKeys, files.EpisodeKey{Episode: ep}) {
			t.Errorf("esperava episódio %d bloqueado, obteve %v", ep, fm.blockedEpisodeKeys)
		}
	}
}

// TestRemoveTorrentWithEpisodes_LoadErrorAbortsWithoutRemoving verifica que uma falha em
// LoadSavedEpisodes retorna erro e não chama backend.Remove.
func TestRemoveTorrentWithEpisodes_LoadErrorAbortsWithoutRemoving(t *testing.T) {
	const hash = "5555555555555555555555555555555555555555"
	fm := &failingFM{loadErr: errors.New("disk on fire")}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{})
	if err == nil {
		t.Fatal("esperava erro quando LoadSavedEpisodes falha")
	}
	if _, ok := backend.Get(hash); !ok {
		t.Error("backend.Remove não deve ser chamado quando LoadSavedEpisodes falha")
	}
}

// TestProcessAnimeEpisodes_NoMagnets_SkipsNewEpisodeWebhook: quando nenhuma busca acha torrent,
// o episódio não pode disparar "novo episódio detectado, iniciando download" — esse push saía a
// cada passada do loop (10 em 10 min) para um episódio que nunca começou a baixar, e ainda fazia
// attemptDownloadWithRetries logar "falhou após todas as tentativas" com zero tentativas.
// Só o webhook de falha (motivo: nenhum torrent encontrado) deve sair.
func TestProcessAnimeEpisodes_NoMagnets_SkipsNewEpisodeWebhook(t *testing.T) {
	events := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- string(body)
	}))
	defer srv.Close()

	englishTitle := "Sem Torrent"
	anime := anilist.MediaList{
		Id:       900,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:     900,
			Status: anilist.MediaStatusReleasing,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: []anilist.AiringNode{{ID: 9001, Episode: 1, TimeUntilAiring: -100}},
			},
		},
	}

	noResults := nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int, string) []nyaa.TorrentResult { return nil },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, string, int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(anilist.Title, bool, string) []nyaa.TorrentResult { return nil },
	}

	configs := &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   3,
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
				Body:   "{{title}}",
				Events: []string{"new_episode", "download_failed"},
			}},
		},
	}

	backend := torrents.NewFakeBackend()
	result := processAnimeEpisodes(configs, backend, anime, nil, nil, map[files.EpisodeKey]bool{}, "", noResults)

	if len(result.newEpisodes) > 0 {
		t.Errorf("nenhum episódio deve ser salvo sem magnet, obteve %d", len(result.newEpisodes))
	}
	if len(backend.List()) > 0 {
		t.Errorf("backend.Add não deve ser chamado sem magnet, cliente tem %d torrent(s)", len(backend.List()))
	}

	var got []string
	for range 2 {
		select {
		case body := <-events:
			got = append(got, body)
		case <-time.After(300 * time.Millisecond):
		}
	}
	if len(got) != 1 || got[0] != "Erro no download" {
		t.Errorf("esperava só o webhook de falha, obteve %v", got)
	}
}

// TestFirstEpisodeToConsider cobre a regra "de onde a lista de episódios começa":
// progresso + 1 na lista do usuário, 1 no avulso (que tem progresso 0), recuando para o menor
// episódio já salvo — sem esse recuo, um episódio salvo abaixo do progresso nunca seria
// "checado" e a poda o apagaria ignorando watched_episodes_to_keep.
func TestFirstEpisodeToConsider(t *testing.T) {
	anime := func(progress int) anilist.MediaList {
		return anilist.MediaList{Progress: progress, Media: anilist.Media{Id: 21}}
	}
	saved := func(nums ...int) []files.EpisodeStruct {
		out := make([]files.EpisodeStruct, 0, len(nums))
		for _, n := range nums {
			out = append(out, files.EpisodeStruct{AnimeID: 21, EpisodeNumber: n})
		}
		return out
	}

	cases := []struct {
		name     string
		progress int
		saved    []files.EpisodeStruct
		want     int
	}{
		{"avulso (progresso 0) comeca no 1", 0, nil, 1},
		{"lista do usuario comeca no seguinte ao assistido", 1122, nil, 1123},
		{"recua para o menor episodio salvo", 1122, saved(1120, 1125), 1120},
		{"episodio salvo de OUTRO anime nao conta", 1122, []files.EpisodeStruct{{AnimeID: 99, EpisodeNumber: 3}}, 1123},
		{"registro sem numero nao arrasta a lista para o zero", 5, saved(0), 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstEpisodeToConsider(anime(tc.progress), tc.saved); got != tc.want {
				t.Errorf("quero %d, veio %d", tc.want, got)
			}
		})
	}
}

// TestCheckEpisode_SkipCode: so o limite por anime entra no relatorio. Todo skip normal
// (lista excluida, ja assistido, ainda nao lancado) devolve "" — um anime em dia gera dezenas
// deles por passe, e se entrassem no relatorio os problemas reais se perderiam no ruido.
func TestCheckEpisode_SkipCode(t *testing.T) {
	title := "SkipCode Anime"
	base := anilist.MediaList{
		Id:       400,
		Progress: 2,
		Media: anilist.Media{
			Id:    400,
			Title: anilist.Title{English: &title},
		},
	}

	t.Run("limite por anime atingido", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 2}
		downloaded := 2
		ep := anilist.AiringNode{ID: 1, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, shouldDelete, code := checkEpisode(configs, 2, ep, base, false, &downloaded, false, false, false)

		if shouldDownload || shouldDelete {
			t.Errorf("limite atingido não baixa nem apaga, obteve (%v, %v)", shouldDownload, shouldDelete)
		}
		if code != IssueMaxEpisodesPerAnime {
			t.Errorf("esperava %q, obteve %q", IssueMaxEpisodesPerAnime, code)
		}
	})

	t.Run("já assistido", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 2, Episode: 1, TimeUntilAiring: -100}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("skip normal não entra no relatório, obteve %q", code)
		}
	})

	t.Run("ainda não lançado", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 3, Episode: 9, TimeUntilAiring: 3600}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("skip normal não entra no relatório, obteve %q", code)
		}
	})

	t.Run("episódio baixável", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 4, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if !shouldDownload {
			t.Error("esperava shouldDownload=true")
		}
		if code != "" {
			t.Errorf("episódio baixado não tem motivo de skip, obteve %q", code)
		}
	})
}

// TestSelectEpisodes_CountsLimitSkips: e do resultado FINAL de selectEpisodes que o relatorio
// tira downloaded/pending. Com 10 episodios pendentes e teto 3, sobram 7.
func TestSelectEpisodes_CountsLimitSkips(t *testing.T) {
	title := "Limit Count Anime"
	nodes := make([]anilist.AiringNode, 10)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 500 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	anime := anilist.MediaList{
		Id: 500,
		Media: anilist.Media{
			Id:             500,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
	configs := &files.Config{MaxEpisodesPerAnime: 3}

	sel := selectEpisodes(configs, 3, anime, nodes, map[files.EpisodeKey]bool{}, map[files.EpisodeKey]files.EpisodeStruct{}, map[string]bool{}, nil, nil)

	if len(sel.toDownload) != 3 {
		t.Fatalf("esperava 3 para baixar, obteve %d", len(sel.toDownload))
	}
	if sel.downloaded != 3 {
		t.Errorf("esperava downloaded=3, obteve %d", sel.downloaded)
	}
	if sel.limitSkipped != 7 {
		t.Errorf("esperava limitSkipped=7, obteve %d", sel.limitSkipped)
	}
}

// --- F1: guard de exclusao de pack ---

// packRecords monta N registros de pack (episodios 1..n) de um mesmo anime, todos apontando para
// o mesmo hash e declarando a faixa start..end.
func packRecords(animeID, n, start, end int, hash string) []files.EpisodeStruct {
	eps := make([]files.EpisodeStruct, 0, n)
	for i := 1; i <= n; i++ {
		eps = append(eps, files.EpisodeStruct{
			AnimeID: animeID, EpisodeNumber: i, EpisodeHash: hash,
			IsBatch: true, BatchStart: start, BatchEnd: end,
		})
	}
	return eps
}

func keysOf(eps []files.EpisodeStruct) []files.EpisodeKey {
	keys := make([]files.EpisodeKey, 0, len(eps))
	for _, ep := range eps {
		keys = append(keys, ep.Key())
	}
	return keys
}

// Defeito B: pack de season (1..23) baixado sob o cour 1, que so registrou 11 episodios. Assistir
// o cour 1 poe os 11 registros no delete set, mas os 12 episodios do cour 2 estao no torrent sem
// registro nenhum — apagar levaria o conteudo do cour 2 junto.
func TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent(t *testing.T) {
	const hash = "1111111111111111111111111111111111111111"
	saved := packRecords(108465, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); !ok {
		t.Error("pack que declara 1..23 com so 11 registros tem conteudo sem dono: o torrent deve sobreviver")
	}
}

// Nao-regressao: pack que declara 1..12 com os 12 registros no delete set nao tem conteudo sem
// dono — sai inteiro, como sempre saiu. E isso que libera espaco para o pack seguinte.
func TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack(t *testing.T) {
	const hash = "2222222222222222222222222222222222222222"
	saved := packRecords(108465, 12, 1, 12, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("pack com a faixa inteira registrada e no delete set deve ser removido")
	}
}

// Defeito A: o mesmo passe adota o pack sob um media id novo (cour 2) e poda os registros do cour
// 1. O snapshot que chega em handleSavedEpisodes e PRE-passe, entao os registros novos so estao em
// newEpisodes — sem inclui-los no guard, o torrent recem-adotado e apagado.
//
// Sem faixa declarada (BatchStart 0) de proposito: assim o teste isola o defeito A do defeito B.
func TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent(t *testing.T) {
	const hash = "3333333333333333333333333333333333333333"
	saved := packRecords(108465, 11, 0, 0, hash)
	newEps := packRecords(127720, 12, 0, 0, hash)

	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		keysToDelete:    keysOf(saved),
		checkedEpisodes: append(keysOf(saved), keysOf(newEps)...),
		newEpisodes:     newEps,
	})

	if _, ok := backend.Get(hash); !ok {
		t.Error("o torrent adotado pelos registros novos do mesmo passe deve sobreviver a poda dos antigos")
	}
}
