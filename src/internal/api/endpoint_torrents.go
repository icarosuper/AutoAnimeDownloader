package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strconv"
)

// TorrentResponse is one row of the downloads screen: a torrent's live progress joined with
// the anime/episode it belongs to. A batch torrent covers several episodes but is a single
// torrent, so it appears once, with EpisodeNumber nil and IsBatch true.
type TorrentResponse struct {
	Hash   string `json:"hash" example:"0123456789abcdef0123456789abcdef01234567"`
	Name   string `json:"name" example:"[SubsPlease] Frieren - 07 (1080p).mkv"`
	Status string `json:"status" example:"downloading"`
	// QueuePosition is the 1-based place in the download queue's waiting line; 0 means the
	// torrent is not waiting (active, completed, or paused by the user). Not a pointer: 0
	// already says "not queued" without ambiguity — there is no position 0.
	QueuePosition int `json:"queue_position" example:"3"`
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
// @Description  Returns a live snapshot of every torrent in the embedded client — progress, speed, ETA, peers and status — joined with the anime/episode it belongs to. Batch torrents appear once, with a null episode_number. Responds with an empty list (not an error) when no session exists yet, i.e. before completed_anime_path is configured.
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

		// List returns nil when there is no session yet (no completed_anime_path configured). That is a
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
		QueuePosition:    t.QueuePosition,
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

// TorrentFileResponse is one file inside a torrent. Path is the RAW relative path from the
// metadata — the whole point of the panel is reading fansub, resolution and codec off it, so
// it is never rewritten into "Ep NN".
type TorrentFileResponse struct {
	Path string `json:"path" example:"[Judas] Frieren - S01E03.mkv"`
	Size int64  `json:"size" example:"1490000000"`
	// BytesCompleted is null when per-file progress is unknown (paused torrent: rain frees the
	// pieces, so FileStats() fails while Files() still answers). null is "unknown", rendered as
	// "—"; 0 would be the lie "nothing downloaded".
	BytesCompleted *int64 `json:"bytes_completed" example:"780000000"`
	// Codec is the video codec read from the FILE'S OWN HEADER (Matroska/EBML), not from its
	// name — most releases simply do not write it in the name. "" (omitted) whenever it cannot
	// be answered: not an .mkv, not finished downloading, or the header is not on disk yet.
	Codec string `json:"codec,omitempty" example:"HEVC"`
	// Episode comes from nyaa.ExtractEpisodeNumber over the FILE NAME — the same cascade the
	// Librarian uses to rename a pack's files. null when nothing matches.
	Episode *int `json:"episode" example:"3"`
}

// @Summary      List a torrent's files
// @Description  Returns the files inside a torrent, in the order the release was built (not re-sorted), with the raw path, size, per-file progress and the episode number extracted from the file name. bytes_completed is null for a paused torrent (rain frees the piece data, so per-file progress is unavailable) and the list is empty while the metadata has not arrived yet.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse{data=[]TorrentFileResponse}
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Failure      500   {object}  SuccessResponse
// @Router       /torrents/{hash}/files [get]
func handleTorrentFiles(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		hash := r.PathValue("hash")
		if hash == "" {
			JSONError(w, http.StatusBadRequest, "INVALID_HASH", "Torrent hash is required")
			return
		}

		// Same presence check as torrentAction: a torrent missing from the session is a clean
		// 404, not a 500 built from the backend's error string. The snapshot is kept for
		// DataDir, which is where the files actually live on disk.
		info, ok := server.Torrents.Get(hash)
		if !ok {
			JSONError(w, http.StatusNotFound, "TORRENT_NOT_FOUND", "Torrent not found")
			return
		}

		list, err := server.Torrents.Files(hash)
		if err != nil {
			JSONInternalError(w, err)
			return
		}

		out := make([]TorrentFileResponse, 0, len(list))
		for _, f := range list {
			out = append(out, TorrentFileResponse{
				Path:           f.Path,
				Size:           f.Size,
				BytesCompleted: f.BytesCompleted,
				// path.Base, não o caminho inteiro: a PASTA do pack costuma carregar a faixa
				// ("… - 00 ~ 12 [1080p]/…"), e a cascata de regexes lida sobre ela devolve o
				// número errado ou nada. Medido no pack do Erai-raws de Mushoku Tensei II: o
				// caminho completo dá nil em TODO arquivo, o nome do arquivo dá o episódio certo.
				Episode: nyaa.ExtractEpisodeNumber(path.Base(f.Path)),
				Codec:   videoCodecOf(info.DataDir, f),
			})
		}

		JSONSuccess(w, http.StatusOK, out)
	}
}

// videoCodecOf reads the codec out of the file's own header, and only when that is both
// possible and cheap: the file has to be COMPLETE, because rain does not download
// sequentially — a file at 90% may well be missing exactly the first pieces, where the
// Matroska header lives. Everything else answers "" and the UI shows nothing.
//
// ponytail: re-read on every poll tick of an open panel (~13 opens of a few KB every 2s for a
// season pack). Negligible next to the torrent traffic itself; if it ever shows up, cache it
// by path — the header of a finished file does not change.
func videoCodecOf(dataDir string, f torrents.FileInfo) string {
	if dataDir == "" || f.BytesCompleted == nil || *f.BytesCompleted < f.Size {
		return ""
	}
	return files.VideoCodec(filepath.Join(dataDir, f.Path))
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
// @Description  Puts a paused torrent at the BACK of the download queue and starts it if a slot is free — with max_concurrent_downloads set, resuming does not mean "start now" (use /prioritize for that). Re-arms the completion listener. A completed (seeding) torrent bypasses the queue.
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

// @Summary      Prioritize a torrent
// @Description  Moves a torrent to the FRONT of the download queue and starts it immediately, pausing the least-progressed active torrent when that would exceed max_concurrent_downloads. Nothing is lost by the demotion — rain keeps the piece bitfield across a stop. A torrent that is already downloading is a no-op; an already-completed one is a 500.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash  path      string  true  "Torrent info hash"
// @Success      200   {object}  SuccessResponse
// @Failure      400   {object}  SuccessResponse
// @Failure      404   {object}  SuccessResponse
// @Failure      405   {object}  SuccessResponse
// @Router       /torrents/{hash}/prioritize [post]
func handleTorrentPrioritize(server *Server) http.HandlerFunc {
	return torrentAction(server, func(s *Server, hash string) error { return s.Torrents.Prioritize(hash) })
}

// PrioritizeRequest is the body of the batch prioritize endpoint.
type PrioritizeRequest struct {
	// Hashes is applied IN THE ORDER RECEIVED — the frontend decides the order, because it is
	// what knows what the user clicked.
	Hashes []string `json:"hashes" example:"0123456789abcdef0123456789abcdef01234567"`
}

// @Summary      Prioritize several torrents at once
// @Description  Moves every listed torrent to the FRONT of the download queue, in the order received, and starts as many as max_concurrent_downloads allows — whatever is pushed past the limit pauses. Unknown or already-completed hashes are ignored rather than rejected: a list of episodes must not fail whole because one of them finished between the render and the click. Use this instead of N calls to /torrents/{hash}/prioritize, which would reverse the batch.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        request  body      PrioritizeRequest  true  "Info hashes, most important first"
// @Success      200      {object}  SuccessResponse
// @Failure      400      {object}  SuccessResponse
// @Failure      405      {object}  SuccessResponse
// @Failure      500      {object}  SuccessResponse
// @Router       /torrents/prioritize [post]
func handleTorrentsPrioritize(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed")
			return
		}

		var req PrioritizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}
		if len(req.Hashes) == 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_BODY", "At least one hash is required")
			return
		}

		if err := server.Torrents.PrioritizeAll(req.Hashes); err != nil {
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, nil)
	}
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

// parseBoolQueryParam reads a boolean query parameter, defaulting to false when absent
// (per the endpoint's contract: keep_data and block both default to false). Any value
// strconv.ParseBool rejects is reported as an error for the caller to turn into a 400.
func parseBoolQueryParam(r *http.Request, name string) (bool, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid value for %s: %q", name, raw)
	}
	return v, nil
}

// @Summary      Delete a torrent
// @Description  Removes a torrent and every saved episode sharing its hash, as a single unit (the deletion boundary is the torrent, not the episode, so a batch's episodes always leave together). By default this frees both the seeding copy and the library hardlink (same inode); keep_data=true keeps both instead. block=true additionally blocks every episode in the group against automatic re-download.
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        hash       path   string  true   "Torrent info hash"
// @Param        keep_data  query  bool    false  "Keep the seeding copy and library hardlinks on disk (default false)"
// @Param        block      query  bool    false  "Block the torrent's episodes against automatic re-download (default false)"
// @Success      200  {object}  SuccessResponse
// @Failure      400  {object}  SuccessResponse
// @Failure      404  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /torrents/{hash} [delete]
//
// Routing note: this is registered on the same "/api/v1/torrents/{hash}" mux pattern for
// every HTTP method — a Go 1.22+ ServeMux pattern with no method prefix matches all verbs on
// that path — so the method check below is what turns a non-DELETE request into a 405
// instead of letting an unmatched pattern 404. Same shape as handleDeleteEpisode.
//
// Deliberate edge case: whether the torrent "exists" is decided only by its presence in the
// client session (server.Torrents.Get), exactly like torrentAction. If the torrent is not in
// the session but saved-episode records with that hash still exist, this responds 404 and
// leaves those records alone — cleaning up an orphaned episode record is the episode flow's
// job (DELETE /animes/{id}/episodes/{episodeId}), not this route's.
func handleTorrentDelete(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is allowed")
			return
		}

		hash := r.PathValue("hash")
		if hash == "" {
			JSONError(w, http.StatusBadRequest, "INVALID_HASH", "Torrent hash is required")
			return
		}

		keepData, err := parseBoolQueryParam(r, "keep_data")
		if err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error())
			return
		}

		block, err := parseBoolQueryParam(r, "block")
		if err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error())
			return
		}

		if _, ok := server.Torrents.Get(hash); !ok {
			JSONError(w, http.StatusNotFound, "TORRENT_NOT_FOUND", "Torrent not found")
			return
		}

		opts := daemon.RemoveTorrentOptions{KeepData: keepData, Block: block}
		if err := daemon.RemoveTorrentWithEpisodes(server.FileManager, server.Torrents, server.Librarian, hash, opts); err != nil {
			logger.Logger.Error().Err(err).Str("hash", hash).Msg("Failed to remove torrent")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, nil)
	}
}
