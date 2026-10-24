package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"
)

type TorrentCandidate struct {
	Name       string `json:"name"`
	Magnet     string `json:"magnet"`
	SizeBytes  int64  `json:"size_bytes"`
	Seeders    int    `json:"seeders"`
	Leechers   int    `json:"leechers"`
	Fansub     string `json:"fansub"`
	Resolution string `json:"resolution"`
	IsBatch    bool   `json:"is_batch"`
	RangeStart int    `json:"range_start"`
	RangeEnd   int    `json:"range_end"`
	NyaaID     int    `json:"nyaa_id"`
}

type TorrentCandidatesResponse struct {
	Query string             `json:"query"`
	Items []TorrentCandidate `json:"items"`
}

type TorrentFile struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

type TorrentCandidateDetail struct {
	DescriptionHTML string        `json:"description_html"`
	Files           []TorrentFile `json:"files"`
	NyaaURL         string        `json:"nyaa_url"`
}

func handleTorrentCandidates(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		id, err := strconv.Atoi(r.PathValue("id"))
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

		used, items, err := daemon.PreviewWholeTitleTorrents(config, id, r.URL.Query().Get("q"))
		if err != nil {
			if errors.Is(err, nyaa.ErrNyaaUnavailable) {
				JSONError(w, http.StatusBadGateway, "NYAA_UNAVAILABLE", "Nyaa is unavailable")
				return
			}
			if strings.Contains(err.Error(), "does not exist on AniList") {
				JSONError(w, http.StatusNotFound, "ANIME_NOT_FOUND", "AniList does not know this media id")
				return
			}
			logger.Logger.Error().Err(err).Int("media_id", id).Msg("Failed to preview torrent candidates")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, TorrentCandidatesResponse{
			Query: used,
			Items: mapTorrentCandidates(items),
		})
	}
}

func handleTorrentCandidateDetail(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_ID", "Invalid anime ID")
			return
		}

		nyaaID, err := strconv.Atoi(r.PathValue("nyaaId"))
		if err != nil || nyaaID <= 0 {
			JSONError(w, http.StatusBadRequest, "INVALID_NYAA_ID", "Invalid Nyaa torrent id")
			return
		}

		detail, err := nyaa.FetchTorrentDetail(nyaaID)
		if err != nil {
			if errors.Is(err, nyaa.ErrTorrentNotFound) {
				JSONError(w, http.StatusNotFound, "TORRENT_NOT_FOUND", "Torrent not found on Nyaa")
				return
			}
			if errors.Is(err, nyaa.ErrNyaaUnavailable) {
				JSONError(w, http.StatusBadGateway, "NYAA_UNAVAILABLE", "Nyaa is unavailable")
				return
			}
			logger.Logger.Error().Err(err).Msg("Failed to fetch torrent detail")
			JSONInternalError(w, err)
			return
		}

		JSONSuccess(w, http.StatusOK, mapTorrentCandidateDetail(detail))
	}
}

func mapTorrentCandidates(in []daemon.TorrentCandidate) []TorrentCandidate {
	out := make([]TorrentCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, TorrentCandidate{
			Name:       c.Name,
			Magnet:     c.Magnet,
			SizeBytes:  c.SizeBytes,
			Seeders:    c.Seeders,
			Leechers:   c.Leechers,
			Fansub:     c.Fansub,
			Resolution: c.Resolution,
			IsBatch:    c.IsBatch,
			RangeStart: c.RangeStart,
			RangeEnd:   c.RangeEnd,
			NyaaID:     c.NyaaID,
		})
	}
	return out
}

func mapTorrentCandidateDetail(d nyaa.TorrentDetail) TorrentCandidateDetail {
	files := make([]TorrentFile, 0, len(d.Files))
	for _, f := range d.Files {
		files = append(files, TorrentFile{Name: f.Name, SizeBytes: f.SizeBytes})
	}
	return TorrentCandidateDetail{
		DescriptionHTML: d.DescriptionHTML,
		Files:           files,
		NyaaURL:         d.NyaaURL,
	}
}
