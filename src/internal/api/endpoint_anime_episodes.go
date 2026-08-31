package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
	"strconv"
	"time"
)

type AnimeEpisodeInfo struct {
	// EpisodeNumber e a identidade do episodio nas rotas /animes/{id}/episodes/{episodeNumber}/*:
	// o id de no da AniList saiu de cena porque nao existe para episodio fora da janela de agenda
	// que ela guarda (ver decisions.md #52).
	EpisodeNumber     int    `json:"episode_number"`
	AiringAt          int64  `json:"airing_at"`
	TimeUntilAiring   int    `json:"time_until_airing"`
	IsAired           bool   `json:"is_aired"`
	IsWatched         bool   `json:"is_watched"`
	IsDownloaded      bool   `json:"is_downloaded"`
	DownloadDate      string `json:"download_date,omitempty"`
	IsManuallyManaged bool   `json:"is_manually_managed,omitempty"`
	IsBlocked         bool   `json:"is_blocked,omitempty"`
	// EpisodeHash is the saved episode's torrent info hash. It is what lets the anime detail
	// screen join in live torrent progress exactly: a batch torrent's saved episodes all carry
	// the same hash but episode_number is meaningless for matching a torrent to them (the
	// torrent itself has no single episode number), so joining by number would miss batches
	// entirely. Joining by hash instead, single and batch episodes use the same path. Empty
	// (and omitted via omitempty) when the episode has no saved record.
	EpisodeHash string `json:"episode_hash,omitempty"`
	// BatchStart/BatchEnd sao a faixa REAL do pack a que este episodio pertence, lida do nome do
	// torrent no download. A tela nao consegue deduzi-la da lista: os episodios ja assistidos nao
	// viram registro, entao o min/max dos registros de um mesmo hash encolhe o pack (um 01-11
	// baixado com 5 assistidos aparecia como "6-11"). Pack sem faixa no nome ("(Season 1+OVA)
	// [Batch]") conta como completo e vale 1..total. Omitidas quando 0 — registro antigo, ou nem
	// o total do AniList e conhecido —, e ai o min/max volta a ser o melhor palpite.
	BatchStart int `json:"batch_start,omitempty"`
	BatchEnd   int `json:"batch_end,omitempty"`
}

type AnimeDetailResponse struct {
	// AnimeID e o id da MIDIA na AniList — a mesma chave usada em episodes.json e nas rotas
	// /animes/{id}/*, e tambem o id do link anilist.co/anime/{id} (ver decisions.md #43).
	AnimeID           int                `json:"anime_id"`
	TotalEpisodes     int                `json:"total_episodes"`
	Progress          int                `json:"progress"`
	Status            string             `json:"status"`
	CoverImage        string             `json:"cover_image,omitempty"`
	Episodes          []AnimeEpisodeInfo `json:"episodes"`
}

// @Summary      Get detail and episodes for a specific anime
// @Description  Returns anime info from AniList combined with downloaded episodes data
// @Tags         animes
// @Accept       json
// @Produce      json
// @Param        id path int true "Anime ID (AniList media ID)"
// @Success      200  {object}  SuccessResponse{data=AnimeDetailResponse}
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /animes/{id}/episodes [get]
func handleAnimeEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		mediaList, err := resolveMediaList(server.FileManager, id, config.AnilistUsernames, loadStandaloneSet(server.FileManager))
		if err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to fetch anime detail from AniList")
			JSONInternalError(w, err)
			return
		}
		if mediaList == nil {
			JSONError(w, http.StatusNotFound, "ANIME_NOT_FOUND", "Anime is not in any configured AniList account")
			return
		}

		allEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		blockedKeys, err := server.FileManager.LoadBlockedEpisodes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to load blocked episodes")
			blockedKeys = []files.EpisodeKey{}
		}
		blockedSet := make(map[files.EpisodeKey]bool, len(blockedKeys))
		for _, k := range blockedKeys {
			blockedSet[k] = true
		}

		type downloadedInfo struct {
			date            string
			manuallyManaged bool
			hash            string
			batchStart      int
			batchEnd        int
		}
		downloadedByEpisode := make(map[int]downloadedInfo)
		for _, ep := range allEpisodes {
			if ep.AnimeID == id {
				downloadedByEpisode[ep.EpisodeNumber] = downloadedInfo{
					date:            ep.DownloadDate.Format(time.RFC3339),
					manuallyManaged: ep.ManuallyManaged,
					hash:            ep.EpisodeHash,
					batchStart:      ep.BatchStart,
					batchEnd:        ep.BatchEnd,
				}
			}
		}

		// A lista vem do builder, e nao do airingSchedule cru: a tela precisa mostrar tambem os
		// episodios que a AniList ja tirou da agenda (One Piece 1 a 1122, anime antigo inteiro),
		// senao o usuario nao tem onde clicar para baixa-los a mao.
		allNodes := anilist.EpisodeList(*mediaList, 1)
		episodes := make([]AnimeEpisodeInfo, 0, len(allNodes))

		for _, node := range allNodes {
			info := AnimeEpisodeInfo{
				EpisodeNumber:   node.Episode,
				AiringAt:        node.AiringAt,
				TimeUntilAiring: node.TimeUntilAiring,
				IsAired:         node.TimeUntilAiring <= 0,
				IsWatched:       node.Episode <= mediaList.Progress,
				IsBlocked:       blockedSet[files.EpisodeKey{AnimeID: id, Episode: node.Episode}],
			}

			if downloaded, ok := downloadedByEpisode[node.Episode]; ok {
				info.IsDownloaded = true
				info.DownloadDate = downloaded.date
				info.IsManuallyManaged = downloaded.manuallyManaged
				info.EpisodeHash = downloaded.hash
				info.BatchStart = downloaded.batchStart
				info.BatchEnd = downloaded.batchEnd
			}

			episodes = append(episodes, info)
		}

		totalEpisodes := 0
		if mediaList.Media.Episodes != nil {
			totalEpisodes = *mediaList.Media.Episodes
		}

		coverImage := mediaList.Media.CoverImage.Large
		if coverImage == "" {
			coverImage = mediaList.Media.CoverImage.Medium
		}

		response := AnimeDetailResponse{
			AnimeID:           id,
			TotalEpisodes:     totalEpisodes,
			Progress:          mediaList.Progress,
			Status:            string(mediaList.Status),
			CoverImage:        coverImage,
			Episodes:          episodes,
		}

		JSONSuccess(w, http.StatusOK, response)
	}
}
