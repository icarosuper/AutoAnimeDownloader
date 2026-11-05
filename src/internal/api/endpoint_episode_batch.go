package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
)

// batchEpisodesRequest is the body of the batch endpoints: episode numbers of ONE anime.
type batchEpisodesRequest struct {
	Episodes []int `json:"episodes"`
}

// batchEpisodeFailure is one episode a batch redownload could not start.
type batchEpisodeFailure struct {
	Episode int    `json:"episode"`
	Error   string `json:"error"`
}

// batchEpisodeKeys validates the path id and the body, and answers the error itself when either is
// bad. Duplicates are dropped and the numbers sorted, so the log and the response read in order.
func batchEpisodeKeys(w http.ResponseWriter, r *http.Request) (int, []files.EpisodeKey, bool) {
	animeId, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || animeId <= 0 {
		JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
		return 0, nil, false
	}
	var req batchEpisodesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Episodes) == 0 {
		JSONError(w, http.StatusBadRequest, "INVALID_BODY", `Body must be {"episodes": [<number>, ...]}`)
		return 0, nil, false
	}
	numbers := slices.Clone(req.Episodes)
	slices.Sort(numbers)
	numbers = slices.Compact(numbers)
	keys := make([]files.EpisodeKey, 0, len(numbers))
	for _, n := range numbers {
		if n <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_EPISODE_NUMBER", "Episode numbers must be positive")
			return 0, nil, false
		}
		keys = append(keys, files.EpisodeKey{AnimeID: animeId, Episode: n})
	}
	return animeId, keys, true
}

// handleBatchDeleteEpisodes is the manual delete for N episodes of one anime: same result as N
// DELETE /episodes/{n} (record gone, library hardlink and torrent freed, episode blocked), in ONE
// removal. The selection used to fire the N calls in parallel, and each one could load the file
// before the others had written: every call still saw the other episodes of a shared pack, the
// pack guard kept the torrent every time, and a pack deleted from the selection stayed on disk.
//
// All or nothing on validation: an episode that is not downloaded fails the request before
// anything is removed, the same 404 the single delete answers.
func handleBatchDeleteEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}
		animeId, keys, ok := batchEpisodeKeys(w, r)
		if !ok {
			return
		}

		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}
		saved := make(map[files.EpisodeKey]bool, len(savedEpisodes))
		for _, ep := range savedEpisodes {
			saved[ep.Key()] = true
		}
		var missing []int
		for _, k := range keys {
			if !saved[k] {
				missing = append(missing, k.Episode)
			}
		}
		if len(missing) > 0 {
			JSONError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", fmt.Sprintf("Episodes not in the downloaded list: %v", missing))
			return
		}

		if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, keys); err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episodes", len(keys)).Msg("Failed to remove episodes")
			JSONInternalError(w, err)
			return
		}
		for _, k := range keys {
			if err := server.FileManager.BlockEpisode(k); err != nil {
				logger.Logger.Warn().Err(err).Int("episode", k.Episode).Msg("Failed to block episode")
			}
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("episodes", len(keys)).Msg("Manually deleted episodes")
		JSONSuccess(w, http.StatusOK, map[string]any{"message": "Episodes deleted", "count": len(keys)})
	}
}

// handleBatchRedownloadEpisodes is POST /episodes/{n}/redownload for N episodes of one anime. The
// downloaded ones are removed in one call first and the request aborts if that fails — the same
// rule as the single redownload: a surviving record would shadow the new torrent (JobOrganize joins
// saved episodes by hash).
//
// After that, one episode failing does not stop the others: each one that started is already
// recorded, so stopping would only leave the rest removed for no reason. The failures come back in
// the response. A failed episode is left unblocked and not downloaded, which the next pass picks up
// on its own.
func handleBatchRedownloadEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}
		animeId, keys, ok := batchEpisodeKeys(w, r)
		if !ok {
			return
		}

		configs, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load configs")
			JSONInternalError(w, err)
			return
		}
		savedEpisodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}
		saved := make(map[files.EpisodeKey]bool, len(savedEpisodes))
		for _, ep := range savedEpisodes {
			saved[ep.Key()] = true
		}
		var downloaded []files.EpisodeKey
		for _, k := range keys {
			if saved[k] {
				downloaded = append(downloaded, k)
			}
		}
		if err := daemon.RemoveEpisodesWithLinks(server.FileManager, server.Torrents, server.Librarian, downloaded); err != nil {
			logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episodes", len(downloaded)).Msg("Failed to remove episodes before redownload")
			JSONInternalError(w, err)
			return
		}

		redownloaded := []int{}
		failed := []batchEpisodeFailure{}
		for _, k := range keys {
			if err := server.FileManager.UnblockEpisode(k); err != nil {
				logger.Logger.Warn().Err(err).Int("episode", k.Episode).Msg("Failed to unblock episode")
			}
			ep, err := daemon.ManualDownloadEpisode(server.FileManager, server.Torrents, animeId, k.Episode, configs)
			if err == nil {
				// UpsertEpisodes for the same reason as the single redownload (decision 27).
				err = server.FileManager.UpsertEpisodes([]files.EpisodeStruct{ep})
			}
			if err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", animeId).Int("episode", k.Episode).Msg("Failed to redownload episode")
				failed = append(failed, batchEpisodeFailure{Episode: k.Episode, Error: err.Error()})
				continue
			}
			redownloaded = append(redownloaded, k.Episode)
		}

		logger.Logger.Info().Int("anime_id", animeId).Int("redownloaded", len(redownloaded)).Int("failed", len(failed)).Msg("Batch redownload finished")
		JSONSuccess(w, http.StatusOK, map[string]any{"redownloaded": redownloaded, "failed": failed})
	}
}
