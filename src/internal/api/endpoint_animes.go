package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// reStripEpisodeNumber strips common episode-numbering suffixes from an episode name.
var reStripEpisodeNumber = []*regexp.Regexp{
	regexp.MustCompile(`\s*-\s*[Ee]pisode\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*[Ee]p\s*\d+.*$`),
	regexp.MustCompile(`\s*-\s*\d+.*$`),
	regexp.MustCompile(`\s+\d+.*$`),
	regexp.MustCompile(`\s*\(.*\)\s*$`),
}

type AnimeInfo struct {
	AnimeID            int    `json:"anime_id" example:"12345"`
	Name               string `json:"name" example:"Naruto"`
	EpisodesDownloaded int    `json:"episodes_downloaded" example:"8"`
	EpisodesReleased   int    `json:"episodes_released" example:"10"`
	EpisodesWatched    int    `json:"episodes_watched" example:"5"`
	// EpisodesPending conta episodios ja lancados, ainda nao assistidos e ainda nao baixados.
	// Episodio assistido nunca e baixado (daemon.shouldSkipEpisode), entao ele nao pode contar
	// como atraso — era isso que marcava meia lista como "atrasado" no frontend.
	EpisodesPending     int    `json:"episodes_pending" example:"2"`
	TotalEpisodes       int    `json:"total_episodes" example:"12"`
	LatestEpisodeNumber int    `json:"latest_episode_number" example:"12"`
	LastDownloadDate    string `json:"last_download_date" example:"2026-02-24T10:30:00Z"`
	CoverImage          string `json:"cover_image,omitempty"`
	IsBlacklisted       bool   `json:"is_blacklisted,omitempty"`
	// IsStandalone marca os animes acompanhados pelo arquivo standalone_animes, e nao por uma
	// lista da AniList. E ORIGEM, nao estado de download — por isso a tela o mostra num chip
	// proprio ao lado do chip derivado, nunca dentro dele.
	IsStandalone bool `json:"is_standalone,omitempty"`
	// NextAiringAt e o unix timestamp da estreia do proximo episodio, 0 quando nao ha nenhum
	// agendado (anime terminado/cancelado). E o TIMESTAMP de proposito, e nao o timeUntilAiring
	// da AniList: o front conta para tras sozinho, entao o numero nao congela junto com a
	// resposta em cache.
	NextAiringAt int64 `json:"next_airing_at,omitempty" example:"1740394800"`
	// AltNames sao os outros titulos do mesmo anime (romaji, ingles, nativo/kanji e sinonimos),
	// sem o proprio Name. Existe so para a busca da dashboard casar com o nome que o usuario
	// conhece; nada renderiza esta lista.
	AltNames []string `json:"alt_names,omitempty" example:"ワンピース"`
}

// altNames junta todas as variantes de titulo de um Media, tirando vazios, repetidos e o nome
// que ja vai em AnimeInfo.Name. Sem normalizacao — quem compara e o front (lib/utils/status.ts).
func altNames(media anilist.Media, displayName string) []string {
	candidates := make([]string, 0, 3+len(media.Synonyms))
	for _, p := range []*string{media.Title.Romaji, media.Title.English, media.Title.Native} {
		if p != nil {
			candidates = append(candidates, *p)
		}
	}
	candidates = append(candidates, media.Synonyms...)

	seen := map[string]bool{strings.ToLower(strings.TrimSpace(displayName)): true}
	var out []string
	for _, c := range candidates {
		name := strings.TrimSpace(c)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

// nextAiringAt le o timestamp do proximo episodio. nextAiringEpisode e nil em anime terminado,
// e a AniList tambem devolve nil durante o hiato entre temporadas.
func nextAiringAt(next *anilist.AiringNode) int64 {
	if next == nil {
		return 0
	}
	return next.AiringAt
}

func extractAnimeName(episodeName string) string {
	if episodeName == "" {
		return "Unknown"
	}

	result := episodeName
	for _, pattern := range reStripEpisodeNumber {
		result = pattern.ReplaceAllString(result, "")
	}

	result = strings.TrimSpace(result)
	if result == "" {
		return episodeName
	}

	return result
}

// @Summary      Get animes list
// @Description  Returns a list of animes with aggregated episode information
// @Tags         animes
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]AnimeInfo}
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes [get]
func handleAnimes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		// Group episodes by anime.
		// Key: "id:<AnimeID>" when AnimeID is set, otherwise the extracted name (backward compat).
		animeMap := make(map[string]*AnimeInfo)
		// Numeros de episodio ja em disco, por anime — base do EpisodesPending calculado no final,
		// depois que o merge da AniList preencheu released/watched.
		downloadedNums := make(map[string]map[int]bool)

		for _, episode := range episodes {
			var key string
			if episode.AnimeID != 0 {
				key = fmt.Sprintf("id:%d", episode.AnimeID)
			} else {
				key = extractAnimeName(episode.EpisodeName)
			}

			// Prefer the persisted AnimeName; fall back to extracting from EpisodeName.
			displayName := episode.AnimeName
			if displayName == "" {
				displayName = extractAnimeName(episode.EpisodeName)
			}

			if episode.EpisodeNumber > 0 {
				if downloadedNums[key] == nil {
					downloadedNums[key] = make(map[int]bool)
				}
				downloadedNums[key][episode.EpisodeNumber] = true
			}

			if animeInfo, exists := animeMap[key]; exists {
				animeInfo.EpisodesDownloaded++
				if episode.EpisodeNumber > animeInfo.LatestEpisodeNumber {
					animeInfo.LatestEpisodeNumber = episode.EpisodeNumber
				}
				lastDownloadedTime, _ := time.Parse(time.RFC3339, animeInfo.LastDownloadDate)
				if episode.DownloadDate.After(lastDownloadedTime) {
					animeInfo.LastDownloadDate = episode.DownloadDate.Format(time.RFC3339)
					// Update name from the most recent episode that has AnimeName set.
					if episode.AnimeName != "" {
						animeInfo.Name = episode.AnimeName
					}
				}
				if animeInfo.AnimeID == 0 && episode.AnimeID != 0 {
					animeInfo.AnimeID = episode.AnimeID
				}
				if animeInfo.TotalEpisodes == 0 && episode.AnimeTotalEpisodes != 0 {
					animeInfo.TotalEpisodes = episode.AnimeTotalEpisodes
				}
			} else {
				animeMap[key] = &AnimeInfo{
					AnimeID:             episode.AnimeID,
					Name:                displayName,
					EpisodesDownloaded:  1,
					TotalEpisodes:       episode.AnimeTotalEpisodes,
					LatestEpisodeNumber: episode.EpisodeNumber,
					LastDownloadDate:    episode.DownloadDate.Format(time.RFC3339),
				}
			}
		}

		// Merge CURRENT animes from AniList so they remain visible even with 0 downloaded episodes.
		// Tracks which AnimeIDs were covered by the filtered fetch, across all accounts, so that
		// already-downloaded animes whose current status fell outside the allowed sets (and thus
		// weren't covered) can be refreshed individually below instead of disappearing.
		var entries []anilist.MediaList
		covered := make(map[int]bool)
		mergeFailed := false
		for _, username := range config.AnilistUsernames {
			// nil (e nao uma lista vazia) significa que a busca falhou — ver fetchAniListEntries.
			list := fetchAniListEntries(username, config.DownloadStatuses, config.DownloadMediaStatuses)
			if list == nil {
				mergeFailed = true
				continue
			}
			for _, ml := range list {
				covered[ml.Media.Id] = true
			}
			entries = append(entries, list...)
		}

		// Os avulsos entram DEPOIS do dedupe pelo mesmo motivo do daemon: quando o anime tambem
		// esta numa lista, a entrada real (com progresso) tem de vencer a sintetica.
		standaloneSet := loadStandaloneSet(server.FileManager)
		entries = append(anilist.DedupeByMedia(entries), appendStandaloneEntries(server.FileManager, nil, standaloneSet, covered)...)

		mergeAniListAnimes(animeMap, entries, config.ExcludedLists)

		// Com uma busca de lista falhada nao da para saber o que ficou coberto, e tratar
		// "nao coberto" como "precisa refresh" transformaria a falha em um GetAnimeInfo por
		// anime baixado, a cada poll do frontend. Era exatamente esse feedback que empurrava
		// a AniList para 429 — e o 429 fazia a proxima lista falhar, prendendo o ciclo.
		if mergeFailed {
			logger.Logger.Warn().Msg("Skipping orphan refresh: AniList list fetch failed, coverage unknown")
		} else {
			refreshOrphanAnimes(server.FileManager, animeMap, covered, config.ExcludedLists, config.AnilistUsernames, standaloneSet)
		}

		animes := make([]AnimeInfo, 0, len(animeMap))
		for key, animeInfo := range animeMap {
			animeInfo.EpisodesPending = countPendingEpisodes(animeInfo, downloadedNums[key])
			animeInfo.IsStandalone = standaloneSet[animeInfo.AnimeID]
			animes = append(animes, *animeInfo)
		}

		JSONSuccess(w, http.StatusOK, animes)
	}
}

// countPendingEpisodes conta os episodios lancados acima do progresso de leitura que ainda nao
// estao em disco. Percorre por numero (e nao released-downloaded) porque episodios assistidos
// podem continuar salvos (watched_episodes_to_keep) e inflariam a subtracao.
func countPendingEpisodes(info *AnimeInfo, downloaded map[int]bool) int {
	pending := 0
	for n := info.EpisodesWatched + 1; n <= info.EpisodesReleased; n++ {
		if !downloaded[n] {
			pending++
		}
	}
	return pending
}

// refreshOrphanAnimes re-fetches AniList-derived fields for already-downloaded animes whose
// AnimeID wasn't covered by the filtered mergeCurrentAniListAnimes fetch (current list/media
// status fell outside the configured allowed sets). These animes stay visible regardless —
// this only tries to keep their cover/progress/blacklist fields fresh instead of stale/blank.
// A failed refresh is logged and left as-is; it never fails the overall request.
//
// TODA a leitura da AniList aqui e em lote: uma query por conta (a cada 50 ids) e, para os
// avulsos que nenhuma conta acompanha, uma segunda por lote de 50. Era uma requisicao POR ANIME
// orfao, disparada a cada poll de 30s de cada aba aberta — a rajada que decisions.md #65 mediu
// como a maior consumidora do orcamento da AniList, e que o burst limiter dela pune. Foi por
// isso tambem que a versao anterior precisava de um semaforo de concorrencia: com duas queries
// no total, nao ha mais o que limitar.
func refreshOrphanAnimes(fm FileManagerInterface, animeMap map[string]*AnimeInfo, covered map[int]bool, excludedLists []string, usernames []string, standalone map[int]bool) {
	var orphans []*AnimeInfo
	var ids []int
	for _, info := range animeMap {
		if info.AnimeID != 0 && !covered[info.AnimeID] {
			orphans = append(orphans, info)
			ids = append(ids, info.AnimeID)
		}
	}
	if len(orphans) == 0 {
		return
	}

	medias, err := anilist.GetAnimeInfoByIDs(ids, usernames)
	if err != nil {
		// Parcial: o mapa traz o que respondeu, e cada orfao ausente dele fica com os campos que
		// vieram do episodes.json. O fallback de avulso NAO roda neste caso — com uma conta
		// falhada, "nenhuma conta acompanha" e uma conclusao que nao se pode tirar, e tira-la
		// gastaria mais requisicoes justo quando a AniList ja esta reclamando.
		logger.Logger.Warn().Err(err).Int("orphans", len(orphans)).
			Msg("Failed to refresh some orphaned animes, keeping existing data")
	} else {
		refreshStandaloneOrphans(fm, medias, ids, standalone)
	}

	for _, info := range orphans {
		ml := medias[info.AnimeID]
		if ml == nil {
			// Nenhuma conta acompanha mais este anime; os episodios em disco continuam listados
			// com os campos que vieram do episodes.json.
			continue
		}

		name, totalEpisodes, episodesReleased, coverImage, isBlacklisted := computeAnimeFields(
			ml.Media.Title, ml.Media.Status, ml.Media.Episodes, ml.Media.CoverImage, ml.Media.AiringSchedule, ml.CustomLists, excludedLists,
		)

		if name != "" {
			info.Name = name
		}
		if info.TotalEpisodes == 0 {
			info.TotalEpisodes = totalEpisodes
		}
		info.EpisodesReleased = episodesReleased
		info.EpisodesWatched = ml.Progress
		info.CoverImage = coverImage
		info.IsBlacklisted = isBlacklisted
		info.NextAiringAt = nextAiringAt(ml.Media.NextAiringEpisode)
		info.AltNames = altNames(ml.Media, info.Name)
	}
}

// refreshStandaloneOrphans preenche, em medias, os orfaos que nenhuma conta acompanha mas que
// estao no arquivo de avulsos — o mesmo fallback (e a mesma razao) de resolveMediaList, so que
// em lote. Sem ele um avulso fora das listas volta a aparecer sem capa e sem nome.
func refreshStandaloneOrphans(fm FileManagerInterface, medias map[int]*anilist.MediaList, ids []int, standalone map[int]bool) {
	var pending []int
	for _, id := range ids {
		if medias[id] == nil && standalone[id] {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return
	}

	fetched, err := anilist.GetMediaByIDs(pending)
	if err != nil {
		logger.Logger.Warn().Err(err).Int("standalone_orphans", len(pending)).
			Msg("Failed to refresh some standalone orphans, keeping existing data")
	}
	for id, ml := range fetched {
		if ml != nil {
			medias[id] = withStandaloneProgress(fm, ml)
		}
	}
}

// computeAnimeFields derives the AniList-sourced display fields shared by the batch merge loop
// (mergeCurrentAniListAnimes) and the single-anime orphan refresh (refreshOrphanAnimes).
func computeAnimeFields(title anilist.Title, status anilist.MediaStatus, episodes *int, cover anilist.CoverImage, schedule anilist.AiringSchedule, customLists anilist.CustomLists, excludedLists []string) (name string, totalEpisodes, episodesReleased int, coverImage string, isBlacklisted bool) {
	if title.English != nil && *title.English != "" {
		name = *title.English
	} else if title.Romaji != nil {
		name = *title.Romaji
	}

	if episodes != nil {
		totalEpisodes = *episodes
	}

	if len(excludedLists) > 0 {
		excludedSet := make(map[string]bool, len(excludedLists))
		for _, n := range excludedLists {
			excludedSet[n] = true
		}
		isBlacklisted = isInExcludedList(customLists, excludedSet)
	}

	coverImage = cover.Large
	if coverImage == "" {
		coverImage = cover.Medium
	}

	for _, node := range schedule.Nodes {
		if node.TimeUntilAiring <= 0 && node.Episode > episodesReleased {
			episodesReleased = node.Episode
		}
	}
	if episodesReleased == 0 && status == anilist.MediaStatusFinished {
		episodesReleased = totalEpisodes
	}

	return name, totalEpisodes, episodesReleased, coverImage, isBlacklisted
}

// fetchAniListEntries returns one account's AniList entries, filtered by both list status
// (server-side) and media status (here), with customLists overlaid.
// Retorna nil — e nao uma lista vazia — quando a busca falha, para o chamador distinguir
// "nenhum anime coberto" de "cobertura desconhecida"; o refresh de orfaos depende disso.
func fetchAniListEntries(username string, statuses []string, mediaStatuses []string) []anilist.MediaList {
	// Fetch customLists via cached minimal query before the complex query that may null it out.
	clMap := anilist.GetCustomListsMap(username, statuses)

	resp, err := anilist.GetFrontendAnimeList(username, statuses)
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to fetch AniList current animes, skipping merge")
		return nil
	}

	filtered := make([]anilist.MediaList, 0, len(resp.Data.Page.MediaList))
	for i := range resp.Data.Page.MediaList {
		ml := &resp.Data.Page.MediaList[i]
		if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 {
			ml.CustomLists = cl
		}
		if !anilist.MediaStatusAllowed(mediaStatuses, ml.Media.Status) {
			continue
		}
		filtered = append(filtered, *ml)
	}
	return filtered
}

// mergeAniListAnimes merges AniList entries into animeMap so they appear even with 0 downloaded
// episodes. It never removes existing animeMap entries — an anime with downloaded episodes stays
// visible even if its current status falls outside the allowed sets (see refreshOrphanAnimes).
func mergeAniListAnimes(animeMap map[string]*AnimeInfo, filtered []anilist.MediaList, excludedLists []string) {
	// Build map from AnimeID → *AnimeInfo pointer so we can update existing entries
	knownByID := make(map[int]*AnimeInfo)
	for _, info := range animeMap {
		if info.AnimeID != 0 {
			knownByID[info.AnimeID] = info
		}
	}

	for _, ml := range filtered {
		episodes := ml.Media.Episodes
		name, totalEpisodes, episodesReleased, coverImage, isBlacklisted := computeAnimeFields(
			ml.Media.Title, ml.Media.Status, episodes, ml.Media.CoverImage, ml.Media.AiringSchedule, ml.CustomLists, excludedLists,
		)
		if name == "" {
			continue
		}

		if existing, ok := knownByID[ml.Media.Id]; ok {
			existing.Name = name
			if existing.TotalEpisodes == 0 {
				existing.TotalEpisodes = totalEpisodes
			}
			existing.EpisodesReleased = episodesReleased
			existing.EpisodesWatched = ml.Progress
			existing.CoverImage = coverImage
			existing.IsBlacklisted = isBlacklisted
			existing.NextAiringAt = nextAiringAt(ml.Media.NextAiringEpisode)
			existing.AltNames = altNames(ml.Media, name)
			continue
		}

		animeMap[name] = &AnimeInfo{
			AnimeID:          ml.Media.Id,
			Name:             name,
			EpisodesReleased: episodesReleased,
			EpisodesWatched:  ml.Progress,
			TotalEpisodes:    totalEpisodes,
			CoverImage:       coverImage,
			IsBlacklisted:    isBlacklisted,
			NextAiringAt:     nextAiringAt(ml.Media.NextAiringEpisode),
			AltNames:         altNames(ml.Media, name),
		}
	}
}
