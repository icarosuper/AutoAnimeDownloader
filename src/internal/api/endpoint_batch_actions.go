package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"net/http"
	"strconv"
)

// batchEpisodeRequest is the body of the batch delete/redownload endpoints: a list of episode
// numbers for one anime. The web UI's row selection and the CLI both need to act on N episodes
// in one request — N single-episode calls would each fire a remove+re-add and race each other.
type batchEpisodeRequest struct {
	Episodes []int `json:"episodes"`
}

func handleBatchEpisodeDelete(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
			return
		}

		animeId, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || animeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		var body batchEpisodeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Episodes) == 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Body must be {\"episodes\": [<int>, ...]}")
			return
		}

		keys := make([]files.EpisodeKey, 0, len(body.Episodes))
		for _, ep := range body.Episodes {
			if ep <= 0 {
				JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Episode numbers must be positive")
				return
			}
			keys = append(keys, files.EpisodeKey{AnimeID: animeId, Episode: ep})
		}

		if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, keys); err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episodes", len(keys)).Msg("Failed to batch delete episodes")
			JSONInternalError(w, err)
			return
		}

		for _, k := range keys {
			if err := server.FileManager.BlockEpisode(k); err != nil {
				logger.Logger.Warn().Err(err).Int("episode", k.Episode).Msg("Failed to block episode after batch delete")
			}
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episodes", len(keys)).Msg("Batch deleted episodes")
		JSONSuccess(w, http.StatusOK, map[string]any{"message": "Episodes deleted", "count": len(keys)})
	}
}

func handleBatchEpisodeRedownload(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
			return
		}

		animeId, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || animeId <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		var body batchEpisodeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Episodes) == 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Body must be {\"episodes\": [<int>, ...]}")
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}

		saved, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}
		savedSet := make(map[files.EpisodeKey]bool)
		for _, ep := range saved {
			savedSet[ep.Key()] = true
		}

		var keys []files.EpisodeKey
		for _, ep := range body.Episodes {
			if ep <= 0 {
				JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Episode numbers must be positive")
				return
			}
			keys = append(keys, files.EpisodeKey{AnimeID: animeId, Episode: ep})
		}

		// Remove first: a surviving record would make the new download untracked (JobOrganize
		// joins saved episodes by hash). Abort on failure so we never leave a half-state.
		for _, k := range keys {
			if savedSet[k] {
				if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, []files.EpisodeKey{k}); err != nil {
					logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", k.Episode).Msg("Failed to remove episode before batch redownload")
					JSONInternalError(w, err)
					return
				}
			}
			if err := server.FileManager.UnblockEpisode(k); err != nil {
				logger.Logger.Warn().Err(err).Int("episode", k.Episode).Msg("Failed to unblock episode")
			}
		}

		downloaded := 0
		for _, k := range keys {
			ep, err := daemon.ManualDownloadEpisode(server.FileManager, server.Torrents, k.AnimeID, k.Episode, configs)
			if err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", k.Episode).Msg("Failed to redownload episode")
				JSONDownloadError(w, err, "REDOWNLOAD_FAILED")
				return
			}
			if err := server.FileManager.UpsertEpisodes([]files.EpisodeStruct{ep}); err != nil {
				logger.Logger.Error().Err(err).Msg("Failed to save episode to file")
				JSONInternalError(w, err)
				return
			}
			downloaded++
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episodes", downloaded).Msg("Batch redownloaded episodes")
		JSONSuccess(w, http.StatusOK, map[string]any{"message": "Episodes redownload started", "count": downloaded})
	}
}