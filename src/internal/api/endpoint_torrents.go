package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
	"net/http"
	"sort"
)

// TorrentResponse is one row of the downloads screen: a torrent's live progress joined with
// the anime/episode it belongs to. A batch torrent covers several episodes but is a single
// torrent, so it appears once, with EpisodeNumber nil and IsBatch true.
type TorrentResponse struct {
	Hash   string `json:"hash" example:"0123456789abcdef0123456789abcdef01234567"`
	Name   string `json:"name" example:"[SubsPlease] Frieren - 07 (1080p).mkv"`
	Status string `json:"status" example:"downloading"`
	// Completed is piece-derived (TorrentInfo.Completed), not Status-derived — it stays true
	// for a torrent paused after finishing, which Status alone cannot tell apart from a
	// paused, unfinished one. Used to key the list sort instead of Status.
	Completed bool `json:"completed" example:"false"`

	AnimeName string `json:"anime_name,omitempty" example:"Sousou no Frieren"`
	AnimeID   int    `json:"anime_id,omitempty" example:"154587"`
	// EpisodeNumber is null for batch torrents (they map to several episodes).
	EpisodeNumber *int `json:"episode_number"`
	IsBatch       bool `json:"is_batch" example:"false"`

	BytesCompleted int64 `json:"bytes_completed" example:"524288000"`
	// BytesTotal is 0 until the torrent's metadata arrives.
	BytesTotal    int64 `json:"bytes_total" example:"1073741824"`
	BytesUploaded int64 `json:"bytes_uploaded" example:"104857600"`
	// Progress is 0..1: BytesCompleted/BytesTotal normally, falling back to the piece ratio
	// (PiecesHave/PiecesTotal) when BytesCompleted reads 0 with a paused torrent — see
	// buildTorrentResponse. 0 while BytesTotal and PiecesTotal are both unknown.
	Progress      float64 `json:"progress" example:"0.48"`
	DownloadSpeed int     `json:"download_speed" example:"2097152"`
	UploadSpeed   int     `json:"upload_speed" example:"524288"`
	PeersTotal    int     `json:"peers_total" example:"14"`
	// EtaSeconds is null when unknown or infinite.
	EtaSeconds       *int64 `json:"eta_seconds" example:"240"`
	SeededForSeconds int64  `json:"seeded_for_seconds" example:"3600"`
}

// @Summary      List torrents
// @Description  Returns a live snapshot of every torrent in the embedded client — progress, speed, ETA, peers and status — joined with the anime/episode it belongs to. Batch torrents appear once, with a null episode_number. Responds with an empty list (not an error) when no session exists yet, i.e. before save_path is configured.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]TorrentResponse}
// @Failure      405  {object}  SuccessResponse
// @Router       /torrents [get]
func handleTorrents(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		// List returns nil when there is no session yet (no save_path configured). That is a
		// normal, empty state — not an error.
		list := server.Torrents.List()

		// The anime/episode join is best-effort: without it the screen still shows progress,
		// just with the raw torrent name. Failing the whole request over it would blank the
		// downloads screen on an unrelated JSONL problem.
		byHash := map[string][]files.EpisodeStruct{}
		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Failed to load saved episodes for torrent list; returning torrents without anime metadata")
		} else {
			for _, ep := range episodes {
				byHash[ep.EpisodeHash] = append(byHash[ep.EpisodeHash], ep)
			}
		}

		out := make([]TorrentResponse, 0, len(list))
		for _, t := range list {
			out = append(out, buildTorrentResponse(t, byHash[t.Hash]))
		}

		// Deterministic order: unfinished torrents first (that is what the user opened the
		// screen for), then alphabetical. Keyed on Completed, not Status: pausing a
		// finished torrent takes it out of "seeding", and keying on the slug would send a
		// paused-but-complete torrent to the top of the list among genuinely unfinished ones.
		sort.SliceStable(out, func(i, j int) bool {
			iDone := out[i].Completed
			jDone := out[j].Completed
			if iDone != jDone {
				return !iDone
			}
			return out[i].Name < out[j].Name
		})

		JSONSuccess(w, http.StatusOK, out)
	}
}

// buildTorrentResponse merges a torrent snapshot with the episodes that share its info hash.
func buildTorrentResponse(t torrents.TorrentInfo, eps []files.EpisodeStruct) TorrentResponse {
	resp := TorrentResponse{
		Hash:             t.Hash,
		Name:             t.Name,
		Status:           t.Status,
		Completed:        t.Completed,
		BytesCompleted:   t.BytesCompleted,
		BytesTotal:       t.BytesTotal,
		BytesUploaded:    t.BytesUploaded,
		DownloadSpeed:    t.DownloadSpeed,
		UploadSpeed:      t.UploadSpeed,
		PeersTotal:       t.PeersTotal,
		EtaSeconds:       t.ETASeconds,
		SeededForSeconds: t.SeededForSeconds,
	}

	// Pausing a torrent frees rain's piece data (torrent_stop.go's closeData nils t.pieces),
	// which zeroes Bytes.Completed while Bytes.Total survives — so a torrent paused at 60%
	// would otherwise render an empty bar at 0%. The piece bitfield survives Stop(), so
	// PiecesHave/PiecesTotal are the reliable source in exactly that state; fall back to it
	// whenever the byte-based figure looks like it collapsed to zero.
	switch {
	case t.BytesTotal > 0 && t.BytesCompleted > 0:
		resp.Progress = float64(t.BytesCompleted) / float64(t.BytesTotal)
	case t.PiecesTotal > 0:
		resp.Progress = float64(t.PiecesHave) / float64(t.PiecesTotal)
	}

	if len(eps) == 0 {
		return resp
	}

	resp.AnimeName = eps[0].AnimeName
	resp.AnimeID = eps[0].AnimeID
	// More than one episode on the same info hash means a batch, whatever the flag says.
	resp.IsBatch = eps[0].IsBatch || len(eps) > 1
	if !resp.IsBatch {
		n := eps[0].EpisodeNumber
		resp.EpisodeNumber = &n
	}
	return resp
}

// torrentAction is the shared shape of the three per-torrent controls: POST only, hash from
// the path, 404 when the torrent is not in the session, and the backend call last.
//
// The action takes the *Server rather than a bound method value: SetupRoutes runs inside
// Server.Start, and binding server.Torrents.Pause at registration time would freeze whatever
// backend was set then. Resolving it per request keeps the handler honest if the field is
// assigned later (as tests do).
func torrentAction(server *Server, action func(s *Server, hash string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		hash := r.PathValue("hash")
		if hash == "" {
			JSONError(w, http.StatusBadRequest, "INVALID_HASH", "Torrent hash is required")
			return
		}

		// Checked up front so a missing torrent is a clean 404 instead of a 500 built from
		// the backend's error string — the codebase has no sentinel errors to match on.
		if _, ok := server.Torrents.Get(hash); !ok {
			JSONError(w, http.StatusNotFound, "TORRENT_NOT_FOUND", "Torrent not found")
			return
		}

		if err := action(server, hash); err != nil {
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, nil)
	}
}

// @Summary      Pause a torrent
// @Description  Stops a torrent. Does not block: the torrent enters the "stopping" state and only reaches "stopped" up to ~5s later, once the stop event reaches the trackers. The paused state is persisted and survives a daemon restart.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/pause [post]
func handleTorrentPause(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Pause(hash) })
}

// @Summary      Resume a torrent
// @Description  Restarts a paused torrent and re-arms its completion listener.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/resume [post]
func handleTorrentResume(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Resume(hash) })
}

// @Summary      Force a torrent re-announce
// @Description  Re-announces the torrent to all trackers and DHT — the way out of "stuck at 0 peers". It does not override the trackers' minimum interval, so repeated calls have no extra effect.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/announce [post]
func handleTorrentAnnounce(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Announce(hash) })
}
