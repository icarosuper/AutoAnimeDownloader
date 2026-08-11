package files

import (
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const configsFolder = ".autoAnimeDownloader"
const configFileName = "config.json"
const downloadedEpsFileName = "downloaded_episodes"
const blockedEpsFileName = "blocked_episodes"
const animeSettingsFileName = "anime_settings"
const standaloneAnimesFileName = "standalone_animes"

// EpisodeKey identifica um episodio. E (anime, numero do episodio) e nao o id do no de
// airingSchedule da AniList, porque aquele id nao existe para todo episodio: a AniList guarda uma
// janela de agenda por midia e descarta as antigas, entao One Piece 1 a 1122 e todo anime antigo
// nao tinham id nenhum — e portanto nao podiam ser baixados (ver decisions.md #52).
type EpisodeKey struct {
	AnimeID int `json:"anime_id"`
	Episode int `json:"episode"`
}

func (e EpisodeStruct) Key() EpisodeKey {
	return EpisodeKey{AnimeID: e.AnimeID, Episode: e.EpisodeNumber}
}

type EpisodeStruct struct {
	AnimeID            int       `json:"anime_id"`
	AnimeTotalEpisodes int       `json:"anime_total_episodes,omitempty"`
	AnimeName          string    `json:"anime_name,omitempty"`
	EpisodeHash        string    `json:"episode_hash"`
	EpisodeName        string    `json:"episode_name"`
	EpisodeNumber      int       `json:"episode_number"`
	DownloadDate       time.Time `json:"download_date"`
	ManuallyManaged    bool      `json:"manually_managed,omitempty"`
	// IsBatch marks episodes that came from a batch/movie torrent (multiple episodes share
	// one EpisodeHash; library files keep raw names, never Jellyfin-renamed).
	IsBatch bool `json:"is_batch,omitempty"`
	// LibraryPaths are the hardlink paths created in the completed-anime library by
	// JobOrganize. Empty means "not yet organized" — the marker JobOrganize uses to fire
	// the completion webhook and write-back exactly once (idempotent across restarts).
	LibraryPaths []string `json:"library_paths,omitempty"`
}

type WebhookPreset struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Events  []string          `json:"events"`
}

type NotificationsConfig struct {
	Webhooks []WebhookPreset `json:"webhooks"`
	// BatchWindowSeconds agrupa os eventos de uma mesma janela num webhook so. 0 desliga o
	// agrupamento (um webhook por evento, comportamento original). Existe porque um backfill
	// de biblioteca dispara um `download_completed` por torrent e estoura a cota de servicos
	// como o ntfy.sh — ver decisions.md #47.
	BatchWindowSeconds int `json:"batch_window_seconds"`
}

type Config struct {
	// SavePath e um campo LEGADO, lido apenas por daemon.MigrateSavePath. O diretorio de
	// download deixou de ser configuravel e passou a ser derivado (ver DownloadPath). O
	// omitempty faz o campo sumir do config.json assim que a migracao o zera.
	SavePath           string   `json:"save_path,omitempty" swaggerignore:"true"`
	CompletedAnimePath string   `json:"completed_anime_path"`
	AnilistUsername    string   `json:"anilist_username,omitempty"`
	AnilistUsernames   []string `json:"anilist_usernames"`
	CheckInterval      int      `json:"check_interval"`
	// MaxEpisodesPerAnime limita quantos episodios de um anime existem ao mesmo tempo, e vale
	// APENAS no caminho episodio-a-episodio: um batch e um torrent so, entao limitar registros
	// nao limitaria bytes nem arquivos na biblioteca (ver decisions.md).
	MaxEpisodesPerAnime int `json:"max_episodes_per_anime"`
	// MaxBatchEpisodes e o teto de episodios para usar batch. Acima disso um anime finalizado
	// volta ao caminho episodio-a-episodio (onde MaxEpisodesPerAnime vale). 0 desliga o teto.
	MaxBatchEpisodes int `json:"max_batch_episodes"`
	// MaxBatchTorrentSizeGB / MaxEpisodeTorrentSizeGB descartam da busca torrents acima do teto,
	// em GiB. 0 desliga (default: nao filtrar, pois nao existe numero defensavel sem saber se o
	// usuario quer 1080p web ou remux).
	MaxBatchTorrentSizeGB   float64 `json:"max_batch_torrent_size_gb"`
	MaxEpisodeTorrentSizeGB float64 `json:"max_episode_torrent_size_gb"`
	// MinFreeDiskPercent barra a adicao de novos torrents abaixo dessa porcentagem de espaco
	// livre no volume da biblioteca. 0 desliga.
	MinFreeDiskPercent int `json:"min_free_disk_percent"`
	EpisodeRetryLimit  int `json:"episode_retry_limit"`
	// MaxConcurrentDownloads limita quantos torrents INCOMPLETOS baixam ao mesmo tempo; o
	// excedente espera na fila (torrents.queue). 0 desliga o limite. Seeding nunca conta.
	//
	// Nao precisa de migracao: LoadConfigs desserializa POR CIMA de getDefaultConfig(),
	// entao um config.json anterior a este campo ja carrega valendo o default.
	MaxConcurrentDownloads int      `json:"max_concurrent_downloads"`
	DeleteWatchedEpisodes  bool     `json:"delete_watched_episodes"`
	WatchedEpisodesToKeep  int      `json:"watched_episodes_to_keep"`
	ExcludedList           string   `json:"excluded_list,omitempty"`
	ExcludedLists          []string `json:"excluded_lists"`
	RenameFilesForJellyfin bool     `json:"rename_files_for_jellyfin"`
	DownloadStatuses       []string `json:"download_statuses"`
	DownloadMediaStatuses  []string `json:"download_media_statuses"`
	DeleteStatuses         []string `json:"delete_statuses"`
	// AnimeIDsAreMediaIDs marca que daemon.MigrateAnimeIDsToMedia ja converteu os AnimeID
	// gravados de id de ENTRADA (MediaList, por conta) para id de MIDIA (ver decisions.md #43).
	// O default e false de proposito: um config.json anterior a este campo desserializa por
	// cima do default e precisa migrar. Numa instalacao nova a migracao roda sem nada a fazer
	// e liga o campo no primeiro passe.
	AnimeIDsAreMediaIDs bool                `json:"anime_ids_are_media_ids"`
	Notifications       NotificationsConfig `json:"notifications"`
	Priorities          nyaa.Priorities     `json:"priorities"`
}

// downloadDirName e o nome do diretorio de download dentro da biblioteca. O ponto o
// esconde do scanner do Jellyfin no Linux; o arquivo .ignore criado por
// Librarian.ProbePath cobre as demais plataformas.
const downloadDirName = ".torrents"

// DownloadPath e o diretorio onde os torrents baixam e continuam semeando. Ele e derivado
// de CompletedAnimePath, nunca armazenado: assim a restricao de hardlink (origem e destino
// no mesmo filesystem) fica impossivel de violar por configuracao.
//
// Devolve "" quando a biblioteca nao esta configurada. Essa guarda e obrigatoria: sem ela
// filepath.Join produziria o caminho relativo ".autoAnimeDownloader" e a sessao da rain
// seria criada no diretorio de trabalho do processo. Com "", SessionManager.Ensure devolve
// ErrSessionNotReady, que e o comportamento atual para config incompleta.
func (c *Config) DownloadPath() string {
	if c.CompletedAnimePath == "" {
		return ""
	}
	return filepath.Join(c.CompletedAnimePath, downloadDirName)
}

type AnimeSettings struct {
	CustomSearchQuery string `json:"custom_search_query,omitempty"`
}

type FileManager struct {
	fs                   FileSystem
	configPath           string
	episodesPath         string
	blockedEpisodesPath  string
	animeSettingsPath    string
	standaloneAnimesPath string
	mu                   sync.Mutex
}

func getDefaultConfig() *Config {
	// Default da biblioteca: ~/Animes. Se o home nao existir (container sem HOME), fica ""
	// e a config segue "incompleta" como antes, exigindo que o usuario preencha.
	completedPath := ""
	if home, err := os.UserHomeDir(); err == nil {
		completedPath = filepath.Join(home, "Animes")
	}

	return &Config{
		SavePath:               "",
		CompletedAnimePath:     completedPath,
		AnilistUsernames:       []string{},
		CheckInterval:          10,
		MaxEpisodesPerAnime:    12,
		MaxBatchEpisodes:       30,
		MinFreeDiskPercent:     10,
		EpisodeRetryLimit:      5,
		MaxConcurrentDownloads: 3,
		DeleteWatchedEpisodes:  true,
		WatchedEpisodesToKeep:  0,
		ExcludedLists:          []string{},
		DownloadStatuses:       []string{"CURRENT", "REPEATING"},
		DownloadMediaStatuses:  []string{"RELEASING", "FINISHED"},
		DeleteStatuses:         []string{},
		Notifications:          NotificationsConfig{Webhooks: []WebhookPreset{}, BatchWindowSeconds: 60},
		Priorities:             nyaa.DefaultPriorities(),
	}
}

func ensureConfigsFolder(fs FileSystem) (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	configsFolderPath := filepath.Join(baseFolder, configsFolder)

	_, err := fs.Stat(configsFolderPath)
	if os.IsNotExist(err) {
		if err := fs.Mkdir(configsFolderPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create configs folder: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to stat configs folder: %w", err)
	}

	return configsFolderPath, nil
}

func NewManager(fs FileSystem, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath, standaloneAnimesPath string) *FileManager {
	return &FileManager{
		fs:                   fs,
		configPath:           configPath,
		episodesPath:         episodesPath,
		blockedEpisodesPath:  blockedEpisodesPath,
		animeSettingsPath:    animeSettingsPath,
		standaloneAnimesPath: standaloneAnimesPath,
	}
}

func NewDefaultFileManager() (*FileManager, error) {
	fs := NewOSFileSystem()
	configsFolderPath, err := ensureConfigsFolder(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure configs folder: %w", err)
	}

	configPath := filepath.Join(configsFolderPath, configFileName)
	episodesPath := filepath.Join(configsFolderPath, downloadedEpsFileName)
	blockedEpisodesPath := filepath.Join(configsFolderPath, blockedEpsFileName)
	animeSettingsPath := filepath.Join(configsFolderPath, animeSettingsFileName)
	standaloneAnimesPath := filepath.Join(configsFolderPath, standaloneAnimesFileName)

	return NewManager(fs, configPath, episodesPath, blockedEpisodesPath, animeSettingsPath, standaloneAnimesPath), nil
}

// ConfigExists diz se ja existe um config.json em disco. Serve para detectar a primeira
// execucao, e por isso precisa ser consultado ANTES do primeiro LoadConfigs — que cria o
// arquivo com os defaults quando ele nao existe.
func (m *FileManager) ConfigExists() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.fs.Stat(m.configPath)
	return err == nil
}

func (m *FileManager) LoadConfigs() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	config := getDefaultConfig()

	_, err := m.fs.Stat(m.configPath)
	if os.IsNotExist(err) {
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		nyaa.SetPriorities(config.Priorities)
		return config, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	file, err := m.fs.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	trimmed := strings.TrimSpace(string(file))
	if len(trimmed) == 0 {
		logger.Logger.Warn().Msg("Config file is empty, recreating with default values")
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		nyaa.SetPriorities(config.Priorities)
		return config, nil
	}

	if err := json.Unmarshal(file, config); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to parse config JSON, recreating with default values")
		config = getDefaultConfig()
		if err := m.saveConfigsLocked(config); err != nil {
			return nil, fmt.Errorf("failed to save default config after parse error: %w", err)
		}
		nyaa.SetPriorities(config.Priorities)
		return config, nil
	}

	// Migrate deprecated excluded_list (string) → excluded_lists ([]string)
	if config.ExcludedList != "" && len(config.ExcludedLists) == 0 {
		for _, item := range strings.Split(config.ExcludedList, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				config.ExcludedLists = append(config.ExcludedLists, trimmed)
			}
		}
		config.ExcludedList = ""
		if err := m.saveConfigsLocked(config); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to save migrated config")
		}
	}

	if config.ExcludedLists == nil {
		config.ExcludedLists = []string{}
	}

	// Migrate deprecated anilist_username (string) → anilist_usernames ([]string)
	if config.AnilistUsername != "" && len(config.AnilistUsernames) == 0 {
		config.AnilistUsernames = []string{config.AnilistUsername}
		config.AnilistUsername = ""
		if err := m.saveConfigsLocked(config); err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to save migrated config")
		}
	}

	if config.AnilistUsernames == nil {
		config.AnilistUsernames = []string{}
	}

	nyaa.SetPriorities(config.Priorities)
	return config, nil
}

func (m *FileManager) SaveConfigs(config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveConfigsLocked(config)
}

// writeAtomic grava via arquivo temporario + rename, para que um leitor (ou uma queda de
// energia no meio da escrita) nunca enxergue o arquivo truncado. Todo estado persistido
// aqui e reescrito por inteiro a cada alteracao, entao um WriteFile direto deixa uma
// janela em que o arquivo esta pela metade — foi assim que o arquivo de episodios corrompeu.
func (m *FileManager) writeAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := m.fs.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tmpPath, err)
	}

	if err := m.fs.Rename(tmpPath, path); err != nil {
		_ = m.fs.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file %s: %w", tmpPath, err)
	}

	return nil
}

// saveConfigsLocked performs an atomic write of config. Must be called with m.mu held.
func (m *FileManager) saveConfigsLocked(config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return m.writeAtomic(m.configPath, data)
}

// As rotinas de episodios abaixo fazem read-modify-write no mesmo arquivo. A UI dispara
// varias delas em paralelo (soltar/apagar varios episodios de uma vez) enquanto o daemon
// organiza torrents, entao todas precisam segurar m.mu — senao as atualizacoes se perdem
// e as escritas concorrentes corrompem o arquivo.
func (m *FileManager) LoadSavedEpisodes() ([]EpisodeStruct, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadSavedEpisodesLocked()
}

// loadSavedEpisodesLocked must be called with m.mu held.
func (m *FileManager) loadSavedEpisodesLocked() ([]EpisodeStruct, error) {
	_, err := m.fs.Stat(m.episodesPath)
	if os.IsNotExist(err) {
		return []EpisodeStruct{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat episodes file: %w", err)
	}

	b, err := m.fs.ReadFile(m.episodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read episodes file: %w", err)
	}

	episodes, err := ParseEpisodes(string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to parse episodes: %w", err)
	}

	if len(episodes) > 0 && episodes[0].DownloadDate.IsZero() {
		needsMigration := false
		for _, ep := range episodes {
			if ep.DownloadDate.IsZero() {
				needsMigration = true
				break
			}
		}
		if needsMigration {
			for i := range episodes {
				if episodes[i].DownloadDate.IsZero() {
					episodes[i].DownloadDate = time.Now()
				}
			}
			if err := m.saveEpisodesLocked(episodes); err != nil {
				return nil, fmt.Errorf("failed to migrate episodes to JSON format: %w", err)
			}
		}
	}

	return episodes, nil
}

func (m *FileManager) SaveEpisodesToFile(episodes []EpisodeStruct) error {
	if len(episodes) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existingEpisodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load existing episodes: %w", err)
	}

	existingMap := make(map[EpisodeKey]bool)
	for _, ep := range existingEpisodes {
		existingMap[ep.Key()] = true
	}

	var newEpisodes []EpisodeStruct
	for _, ep := range episodes {
		if !existingMap[ep.Key()] {
			newEpisodes = append(newEpisodes, ep)
		}
	}

	if len(newEpisodes) == 0 {
		return nil
	}

	allEpisodes := append(existingEpisodes, newEpisodes...)

	return m.saveEpisodesLocked(allEpisodes)
}

// UpsertEpisodes updates existing saved episodes (matched by EpisodeKey) in place and
// appends any that are new. Unlike SaveEpisodesToFile, it overwrites existing records —
// used to write back LibraryPaths after a torrent is organized into the library.
func (m *FileManager) UpsertEpisodes(episodes []EpisodeStruct) error {
	if len(episodes) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load existing episodes: %w", err)
	}

	updates := make(map[EpisodeKey]EpisodeStruct, len(episodes))
	for _, ep := range episodes {
		updates[ep.Key()] = ep
	}

	result := make([]EpisodeStruct, 0, len(existing)+len(episodes))
	seen := make(map[EpisodeKey]bool, len(existing))
	for _, ep := range existing {
		if updated, ok := updates[ep.Key()]; ok {
			result = append(result, updated)
			seen[ep.Key()] = true
		} else {
			result = append(result, ep)
		}
	}
	for _, ep := range episodes {
		if !seen[ep.Key()] {
			result = append(result, ep)
			seen[ep.Key()] = true
		}
	}

	return m.saveEpisodesLocked(result)
}

// saveEpisodesLocked saves episodes in JSONL format. Must be called with m.mu held.
func (m *FileManager) saveEpisodesLocked(episodes []EpisodeStruct) error {
	content, err := SerializeEpisodes(episodes)
	if err != nil {
		return fmt.Errorf("failed to serialize episodes: %w", err)
	}

	if err := m.writeAtomic(m.episodesPath, []byte(content)); err != nil {
		return fmt.Errorf("failed to write episodes to file: %w", err)
	}

	return nil
}

func (m *FileManager) DeleteEpisodesFromFile(keys []EpisodeKey) error {
	if len(keys) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	savedEpisodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return fmt.Errorf("failed to load saved episodes: %w", err)
	}

	if len(savedEpisodes) == 0 {
		return nil
	}

	toDelete := make(map[EpisodeKey]struct{}, len(keys))
	for _, k := range keys {
		toDelete[k] = struct{}{}
	}

	var newSaved []EpisodeStruct
	for _, ep := range savedEpisodes {
		if _, found := toDelete[ep.Key()]; !found {
			newSaved = append(newSaved, ep)
		}
	}

	if len(newSaved) == len(savedEpisodes) {
		return nil
	}

	return m.saveEpisodesLocked(newSaved)
}

// DeleteEmptyFolders remove os diretorios de anime que ficaram vazios na biblioteca depois
// de uma exclusao. O diretorio de download vive dentro dela (ver Config.DownloadPath), e a
// rain aloca <download>/<id> antes de escrever qualquer byte — por isso a varredura o pula
// explicitamente, senao apagaria torrents recem-adicionados.
func (m *FileManager) DeleteEmptyFolders(completedAnimeSaveFolder string) error {
	if completedAnimeSaveFolder == "" {
		return fmt.Errorf("completed anime path cannot be empty")
	}

	if err := m.deleteEmptyFolders(completedAnimeSaveFolder); err != nil {
		return fmt.Errorf("failed to delete empty folders in completed anime save folder: %w", err)
	}

	return nil
}

func (m *FileManager) LoadBlockedEpisodes() ([]EpisodeKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadBlockedEpisodesLocked()
}

// loadIntListLocked le um arquivo de estado que e so um array JSON de ids (standalone_animes).
// Arquivo ausente e lista vazia, nao erro: e o estado inicial normal.
// Must be called with m.mu held.
func (m *FileManager) loadIntListLocked(path, what string) ([]int, error) {
	_, err := m.fs.Stat(path)
	if os.IsNotExist(err) {
		return []int{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat %s file: %w", what, err)
	}

	b, err := m.fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s file: %w", what, err)
	}

	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil {
		return nil, fmt.Errorf("failed to parse %s file: %w", what, err)
	}

	return ids, nil
}

// saveIntListLocked must be called with m.mu held.
func (m *FileManager) saveIntListLocked(path string, ids []int, what string) error {
	if ids == nil {
		ids = []int{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", what, err)
	}
	if err := m.writeAtomic(path, b); err != nil {
		return fmt.Errorf("failed to write %s file: %w", what, err)
	}
	return nil
}

// loadBlockedEpisodesLocked le blocked_episodes, um array JSON de EpisodeKey. Must be called
// with m.mu held.
//
// O arquivo no formato antigo era um array de ids de no da AniList (`[416348, ...]`). Esses ids
// nao existem mais em lugar nenhum do codigo, entao nao ha como converte-los: o arquivo legado e
// DESCARTADO com um aviso, e o usuario perde os bloqueios manuais que tinha. Bloquear e um clique
// por episodio na tela de detalhe; migrar por adivinhacao seria pior que refazer.
func (m *FileManager) loadBlockedEpisodesLocked() ([]EpisodeKey, error) {
	_, err := m.fs.Stat(m.blockedEpisodesPath)
	if os.IsNotExist(err) {
		return []EpisodeKey{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat blocked episodes file: %w", err)
	}

	b, err := m.fs.ReadFile(m.blockedEpisodesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read blocked episodes file: %w", err)
	}

	var keys []EpisodeKey
	if err := json.Unmarshal(b, &keys); err != nil {
		var legacyIDs []int
		if json.Unmarshal(b, &legacyIDs) == nil {
			logger.Logger.Warn().
				Int("count", len(legacyIDs)).
				Msg("Discarding legacy blocked_episodes file: AniList airing-node ids can no longer be resolved to episodes")
			return []EpisodeKey{}, nil
		}
		return nil, fmt.Errorf("failed to parse blocked episodes file: %w", err)
	}

	return keys, nil
}

func (m *FileManager) BlockEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys, err := m.loadBlockedEpisodesLocked()
	if err != nil {
		return err
	}

	for _, k := range keys {
		if k == key {
			return nil // already blocked
		}
	}

	return m.saveBlockedEpisodesLocked(append(keys, key))
}

func (m *FileManager) UnmanageEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	episodes, err := m.loadSavedEpisodesLocked()
	if err != nil {
		return err
	}

	found := false
	for i, ep := range episodes {
		if ep.Key() == key {
			episodes[i].ManuallyManaged = false
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	return m.saveEpisodesLocked(episodes)
}

func (m *FileManager) UnblockEpisode(key EpisodeKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys, err := m.loadBlockedEpisodesLocked()
	if err != nil {
		return err
	}

	filtered := make([]EpisodeKey, 0, len(keys))
	for _, k := range keys {
		if k != key {
			filtered = append(filtered, k)
		}
	}

	if len(filtered) == len(keys) {
		return nil // not found, nothing to do
	}

	return m.saveBlockedEpisodesLocked(filtered)
}

// saveBlockedEpisodesLocked must be called with m.mu held.
func (m *FileManager) saveBlockedEpisodesLocked(keys []EpisodeKey) error {
	if keys == nil {
		keys = []EpisodeKey{}
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("failed to marshal blocked episodes: %w", err)
	}
	if err := m.writeAtomic(m.blockedEpisodesPath, b); err != nil {
		return fmt.Errorf("failed to write blocked episodes file: %w", err)
	}
	return nil
}

func (m *FileManager) loadAllAnimeSettings() (map[int]AnimeSettings, error) {
	_, err := m.fs.Stat(m.animeSettingsPath)
	if os.IsNotExist(err) {
		return map[int]AnimeSettings{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat anime settings file: %w", err)
	}

	b, err := m.fs.ReadFile(m.animeSettingsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read anime settings file: %w", err)
	}

	var settings map[int]AnimeSettings
	if err := json.Unmarshal(b, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse anime settings file: %w", err)
	}

	return settings, nil
}

func (m *FileManager) saveAllAnimeSettings(settings map[int]AnimeSettings) error {
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal anime settings: %w", err)
	}
	if err := m.writeAtomic(m.animeSettingsPath, b); err != nil {
		return fmt.Errorf("failed to write anime settings file: %w", err)
	}
	return nil
}

func (m *FileManager) LoadAnimeSettings(animeID int) (*AnimeSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAllAnimeSettings()
	if err != nil {
		return nil, err
	}

	s, ok := all[animeID]
	if !ok {
		return &AnimeSettings{}, nil
	}
	return &s, nil
}

func (m *FileManager) SaveAnimeSettings(animeID int, settings AnimeSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAllAnimeSettings()
	if err != nil {
		return err
	}

	all[animeID] = settings
	return m.saveAllAnimeSettings(all)
}

func (m *FileManager) LoadAllAnimeSettings() (map[int]AnimeSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadAllAnimeSettings()
}

func (m *FileManager) deleteEmptyFolders(path string) error {
	entries, err := m.fs.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read save path: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if entry.Name() == downloadDirName {
			continue
		}

		folderPath := filepath.Join(path, entry.Name())
		subEntries, err := m.fs.ReadDir(folderPath)
		if err != nil {
			logger.Logger.Warn().
				Err(err).
				Str("folder_path", folderPath).
				Msg("Failed to read folder while deleting empty folders")
			continue
		}

		if len(subEntries) == 0 {
			if err := m.fs.Remove(folderPath); err != nil {
				logger.Logger.Warn().
					Err(err).
					Str("folder_path", folderPath).
					Msg("Failed to delete empty folder")
			} else {
				logger.Logger.Info().
					Str("folder_path", folderPath).
					Msg("Deleted empty folder")
			}
		}
	}

	return nil
}
