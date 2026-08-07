package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
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
	deletedEpisodeIDs []int
	blockedEpisodeIDs []int
	blockErr          error
}

func (m *mockFileManagerForEpisodes) LoadConfigs() (*files.Config, error) { return nil, nil }
func (m *mockFileManagerForEpisodes) SaveConfigs(*files.Config) error     { return nil }
func (m *mockFileManagerForEpisodes) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) SaveEpisodesToFile([]files.EpisodeStruct) error { return nil }
func (m *mockFileManagerForEpisodes) UpsertEpisodes([]files.EpisodeStruct) error     { return nil }
func (m *mockFileManagerForEpisodes) DeleteEpisodesFromFile(ids []int) error {
	m.deletedEpisodeIDs = append(m.deletedEpisodeIDs, ids...)
	return nil
}
func (m *mockFileManagerForEpisodes) DeleteEmptyFolders(string) error     { return nil }
func (m *mockFileManagerForEpisodes) LoadBlockedEpisodes() ([]int, error) { return nil, nil }
func (m *mockFileManagerForEpisodes) BlockEpisode(id int) error {
	m.blockedEpisodeIDs = append(m.blockedEpisodeIDs, id)
	return m.blockErr
}
func (m *mockFileManagerForEpisodes) UnblockEpisode(int) error  { return nil }
func (m *mockFileManagerForEpisodes) UnmanageEpisode(int) error { return nil }
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

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:    episodeID,
			AnimeID:      animeID,
			EpisodeHash:  episodeHash,
			AnimeName:    "Dropped Anime",
			DownloadDate: time.Now(),
		},
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("esperava torrent %q removido do cliente, mas ainda presente", episodeHash)
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

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeID:       episodeID,
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

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     []int{episodeID},
		checkedEpisodes: []int{episodeID},
	}

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), data)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("esperava torrent %q removido do cliente, mas ainda presente", episodeHash)
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

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, nil, savedEpisodes, map[int]bool{}, "", defaultNyaaSearcher())

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

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		idsToDelete:     []int{episodeID},
		checkedEpisodes: []int{episodeID},
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
			EpisodeID:    1000 + i,
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

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, dlTorrents, savedEpisodes, map[int]bool{}, "", mockSearcher)

	if searchBatchCalled {
		t.Error("searchBatch não deve ser chamado: todos os episódios já estão no cliente pelo hash")
	}
	if len(backend.List()) > 0 {
		t.Errorf("Add não deve ser chamado, mas o cliente tem %d torrent(s)", len(backend.List()))
	}
	if len(result.newEpisodes) > 0 {
		t.Errorf("newEpisodes deve estar vazio, obteve %d episódio(s)", len(result.newEpisodes))
	}
	if len(result.idsToDelete) > 0 {
		t.Errorf("idsToDelete deve estar vazio, obteve %v", result.idsToDelete)
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
		if ep.EpisodeID == id {
			return ep
		}
	}
	t.Fatalf("episode %d not found in saved episodes %+v", id, all)
	return files.EpisodeStruct{}
}

// TestSaveEpisodesToFile_RedownloadUpdatesRecord cobre o caminho de upgrade: um registro salvo
// antes do campo EpisodeNumber existir (número 0) e com hash antigo é re-baixado. O registro
// persistido deve ficar com o EpisodeNumber e o EpisodeHash NOVOS — sem isso o JobOrganize não
// acha o torrent pelo hash e o hardlink sai como "Anime - E00.mkv".
func TestSaveEpisodesToFile_RedownloadUpdatesRecord(t *testing.T) {
	fm := tempFileManager(t)
	old := time.Now().Add(-48 * time.Hour)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeID:    42,
		AnimeID:      7,
		AnimeName:    "My Anime",
		EpisodeHash:  oldHash,
		EpisodeName:  "My Anime - Episode 5",
		DownloadDate: old,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeID:          42,
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
		EpisodeID:       42,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		ManuallyManaged: true,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// O loop automático nunca marca ManuallyManaged.
	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeID:     42,
		AnimeID:       7,
		EpisodeHash:   newHash,
		EpisodeNumber: 5,
	}})

	got := loadEpisodeByID(t, fm, 42)
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
		EpisodeID:    42,
		AnimeID:      7,
		EpisodeHash:  oldHash,
		LibraryPaths: []string{"/library/My Anime/My Anime - E05.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeID:     42,
		AnimeID:       7,
		EpisodeHash:   newHash,
		EpisodeNumber: 5,
	}})

	got := loadEpisodeByID(t, fm, 42)
	if len(got.LibraryPaths) != 0 {
		t.Errorf("LibraryPaths deve ser zerado no re-download, obteve %v", got.LibraryPaths)
	}
}

// TestSaveEpisodesToFile_AppendsNewEpisode garante que o merge não regride o comportamento
// atual: episódio genuinamente novo continua sendo apendado sem tocar nos existentes.
func TestSaveEpisodesToFile_AppendsNewEpisode(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeID:       1,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		EpisodeNumber:   1,
		ManuallyManaged: true,
		LibraryPaths:    []string{"/library/My Anime/My Anime - E01.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeID:     2,
		AnimeID:       7,
		EpisodeHash:   newHash,
		EpisodeNumber: 2,
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
	deleted   []int
}

func (m *failingFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.saved, nil
}

func (m *failingFM) DeleteEpisodesFromFile(ids []int) error {
	m.deleted = append(m.deleted, ids...)
	return m.deleteErr
}

// TestRemoveEpisodesWithLinks_PropagatesLoadError: falha ao ler o arquivo de episódios deve
// virar erro para o chamador (a API responde 500) em vez de "sucesso" silencioso.
func TestRemoveEpisodesWithLinks_PropagatesLoadError(t *testing.T) {
	fm := &failingFM{loadErr: errors.New("disk on fire")}
	err := RemoveEpisodesWithLinks(fm, torrents.NewFakeBackend(), testLibrarian(), []int{1})
	if err == nil {
		t.Fatal("esperava erro quando LoadSavedEpisodes falha")
	}
}

// TestRemoveEpisodesWithLinks_PropagatesDeleteError: falha ao remover do arquivo deve virar erro.
func TestRemoveEpisodesWithLinks_PropagatesDeleteError(t *testing.T) {
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	fm := &failingFM{
		saved:     []files.EpisodeStruct{{EpisodeID: 1, EpisodeHash: hash}},
		deleteErr: errors.New("write failed"),
	}
	backend := fakeWithTorrents(hash)

	err := RemoveEpisodesWithLinks(fm, backend, testLibrarian(), []int{1})
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
	fm := &failingFM{saved: []files.EpisodeStruct{{EpisodeID: 1, EpisodeHash: hash}}}
	if err := RemoveEpisodesWithLinks(fm, fakeWithTorrents(hash), testLibrarian(), []int{1}); err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}
	if !containsID(fm.deleted, 1) {
		t.Errorf("esperava episódio 1 deletado do arquivo, obteve %v", fm.deleted)
	}
}

// TestHandleSavedEpisodes_DeleteErrorIsTolerated: o loop automático continua best-effort —
// uma falha ao deletar do arquivo é logada, não aborta o passe nem impede a liberação de espaço.
func TestHandleSavedEpisodes_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	saved := []files.EpisodeStruct{{EpisodeID: 55, AnimeID: 300, EpisodeHash: hash}}
	fm := &failingFM{saved: saved, deleteErr: errors.New("write failed")}
	backend := fakeWithTorrents(hash)

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true, MaxEpisodesPerAnime: 12}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		idsToDelete:     []int{55},
		checkedEpisodes: []int{55},
	})

	if _, ok := backend.Get(hash); ok {
		t.Error("o passe deve continuar e remover o torrent mesmo com falha na escrita do arquivo")
	}
	if !containsID(fm.deleted, 55) {
		t.Errorf("a deleção deve ter sido tentada, obteve %v", fm.deleted)
	}
}

// TestDeleteEpisodesByStatus_DeleteErrorIsTolerated: idem para a deleção por status.
func TestDeleteEpisodesByStatus_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "ffffffffffffffffffffffffffffffffffffffff"
	deletableMedia := map[int]bool{100: true}

	saved := []files.EpisodeStruct{{EpisodeID: 42, AnimeID: 100, EpisodeHash: hash}}
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
		{EpisodeID: 1, EpisodeHash: hash},
		{EpisodeID: 2, EpisodeHash: hash},
		{EpisodeID: 3, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := newCountingBackend(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}

	for _, id := range []int{1, 2, 3} {
		if !containsID(fm.deleted, id) {
			t.Errorf("esperava episódio %d removido do arquivo, obteve %v", id, fm.deleted)
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
		{EpisodeID: 1, EpisodeHash: hash, LibraryPaths: []string{libPath}},
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
	if len(fm.blockedEpisodeIDs) != 0 {
		t.Errorf("torrent órfão não tem episódio para bloquear, obteve %v", fm.blockedEpisodeIDs)
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
		{EpisodeID: 10, EpisodeHash: hash},
		{EpisodeID: 20, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{Block: true})
	if err != nil {
		t.Fatalf("esperava sucesso, obteve %v", err)
	}
	for _, id := range []int{10, 20} {
		if !containsID(fm.blockedEpisodeIDs, id) {
			t.Errorf("esperava episódio %d bloqueado, obteve %v", id, fm.blockedEpisodeIDs)
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
