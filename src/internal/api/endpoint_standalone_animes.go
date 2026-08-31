package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
)

type StandaloneAnimeRequest struct {
	MediaID int `json:"media_id" example:"21"`
}

type StandaloneAnimeAddResponse struct {
	// Added e quantos episodios entraram na fila agora. 0 e resposta normal: um anime
	// NOT_YET_RELEASED (ou um cujo proximo episodio ainda nao foi ao ar) fica acompanhado do
	// mesmo jeito. Quem traduz isso e o toast, nao um codigo de erro.
	Added int `json:"added" example:"3"`
}

// blockReasonToErrorCode traduz o motivo de bloqueio no codigo do 409. Sao os mesmos quatro
// motivos que a busca devolve em block_reason.
var blockReasonToErrorCode = map[string]string{
	blockReasonBlacklist:  "ALREADY_BLACKLISTED",
	blockReasonStandalone: "ALREADY_STANDALONE",
	blockReasonTracked:    "ALREADY_TRACKED",
	blockReasonDownloaded: "ALREADY_DOWNLOADED",
}

func handleStandaloneAnimeAdd(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		var body StandaloneAnimeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MediaID <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Body must be {\"media_id\": <positive int>}")
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		// Primeiro porque nao depende de rede e porque, sem biblioteca, o Ensure la embaixo
		// falharia e o usuario receberia um 500 opaco no lugar da causa.
		if config.DownloadPath() == "" {
			JSONError(w, http.StatusConflict, "LIBRARY_NOT_CONFIGURED",
				"Configure the completed anime path before adding animes")
			return
		}

		// E tambem o que valida que o id existe e e ANIME, e de onde sai o total de episodios
		// que blockReason precisa.
		media, err := anilist.GetMediaByID(body.MediaID, anilist.PriorityCritical)
		if err != nil {
			logger.Logger.Error().Err(err).Int("media_id", body.MediaID).Msg("Failed to fetch media from AniList")
			JSONInternalError(w, err)
			return
		}
		if media == nil {
			JSONError(w, http.StatusNotFound, "ANIME_NOT_FOUND", "AniList does not know this media id")
			return
		}
		totalEpisodes := 0
		if media.Media.Episodes != nil {
			totalEpisodes = *media.Media.Episodes
		}

		guard, err := newStandaloneGuard(server.FileManager, config)
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to build the standalone guard")
			JSONInternalError(w, err)
			return
		}
		if reason := guard.blockReason(body.MediaID, totalEpisodes); reason != "" {
			JSONError(w, http.StatusConflict, blockReasonToErrorCode[reason], standaloneBlockMessage(reason))
			return
		}

		if err := server.FileManager.AddStandaloneAnime(body.MediaID); err != nil {
			logger.Logger.Error().Err(err).Int("media_id", body.MediaID).Msg("Failed to save the standalone anime")
			JSONInternalError(w, err)
			return
		}

		// Sincrono, seguindo o POST de download de episodio: e uma busca so no Nyaa e devolver o
		// numero real de episodios enfileirados vale mais que um 202 vazio.
		added, err := daemon.DownloadStandaloneAnime(server.FileManager, server.Torrents, config, body.MediaID)
		if err != nil {
			logger.Logger.Error().Err(err).Int("media_id", body.MediaID).Msg("Failed to download the standalone anime")
			JSONDownloadError(w, err, "STANDALONE_DOWNLOAD_FAILED")
			return
		}

		JSONSuccess(w, http.StatusOK, StandaloneAnimeAddResponse{Added: added})
	}
}

func standaloneBlockMessage(reason string) string {
	switch reason {
	case blockReasonBlacklist:
		return "This anime is in an excluded AniList list"
	case blockReasonStandalone:
		return "This anime is already tracked as standalone"
	case blockReasonTracked:
		return "This anime is already in one of your AniList lists"
	case blockReasonDownloaded:
		return "This anime is already fully downloaded"
	}
	return "This anime cannot be added"
}

func handleStandaloneAnimeRemove(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is allowed")
			return
		}

		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		deleteEpisodes, err := parseBoolQueryParam(r, "delete_episodes")
		if err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error())
			return
		}

		if err := server.FileManager.RemoveStandaloneAnime(id); err != nil {
			logger.Logger.Error().Err(err).Int("media_id", id).Msg("Failed to remove the standalone anime")
			JSONInternalError(w, err)
			return
		}

		saved, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}

		var group []files.EpisodeStruct
		var keys []files.EpisodeKey
		for _, ep := range saved {
			if ep.AnimeID == id {
				group = append(group, ep)
				keys = append(keys, ep.Key())
			}
		}

		if deleteEpisodes {
			if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, keys); err != nil {
				logger.Logger.Error().Err(err).Int("media_id", id).Msg("Failed to delete the standalone anime episodes")
				JSONInternalError(w, err)
				return
			}
			JSONSuccess(w, http.StatusOK, nil)
			return
		}

		// Sem delete_episodes os arquivos ficam — e o ManuallyManaged e o que os protege. Sem o
		// registro avulso o anime deixa de entrar no passe, entao seus episodios cairiam em
		// identifyEpisodesNotInWatching no ciclo seguinte e sumiriam do disco sem que ninguem
		// tivesse pedido.
		for i := range group {
			group[i].ManuallyManaged = true
		}
		if err := server.FileManager.UpsertEpisodes(group); err != nil {
			logger.Logger.Error().Err(err).Int("media_id", id).Msg("Failed to mark the episodes as manually managed")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, nil)
	}
}
