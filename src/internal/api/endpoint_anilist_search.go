package api

import (
	"net/http"
	"strings"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
)

// minSearchTermLength: shorter terms return an empty list without hitting AniList. One or
// two letters match half the catalog, and AniList's limit is 30 req/min.
const minSearchTermLength = 3

type AniListSearchResult struct {
	Id     int    `json:"id" example:"21"`
	Title  string `json:"title" example:"One Piece"`
	Format string `json:"format" example:"TV"`
	Status string `json:"status" example:"RELEASING"`
	Year   int    `json:"year" example:"1999"`
	// Episodes is 0 when AniList does not know the total (typical for an airing anime).
	Episodes int    `json:"episodes" example:"24"`
	Cover    string `json:"cover,omitempty"`
	// BlockReason is "" when the anime can be added as standalone, otherwise one of the
	// reasons from standalone_guard.go. ONE field, not four booleans: the reasons are mutually
	// exclusive by precedence and the card needs a single label. Comes from the same
	// blockReason the POST uses for the 409 — front and back agree by construction.
	BlockReason string `json:"block_reason"`
}

func handleAniListSearch(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		term := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(term) < minSearchTermLength {
			// Empty list, not 400: a short term is the normal state of someone still typing,
			// not a client error — the frontend already disables the button before we get here.
			JSONSuccess(w, http.StatusOK, []AniListSearchResult{})
			return
		}

		includeUnreleased, err := parseBoolQueryParam(r, "include_unreleased")
		if err != nil {
			JSONError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error())
			return
		}

		config, err := server.FileManager.LoadConfigs()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load config")
			JSONInternalError(w, err)
			return
		}

		results, err := anilist.SearchMedia(term, includeUnreleased)
		if err != nil {
			logger.Logger.Error().Err(err).Str("term", term).Msg("AniList search failed")
			JSONInternalError(w, err)
			return
		}

		guard, err := newStandaloneGuard(server.FileManager, config)
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to build the standalone guard")
			JSONInternalError(w, err)
			return
		}

		out := make([]AniListSearchResult, 0, len(results))
		for _, r := range results {
			episodes := 0
			if r.Episodes != nil {
				episodes = *r.Episodes
			}
			cover := r.CoverImage.Large
			if cover == "" {
				cover = r.CoverImage.Medium
			}
			out = append(out, AniListSearchResult{
				Id:          r.Id,
				Title:       resolveTitle(r.Title),
				Format:      string(r.Format),
				Status:      string(r.Status),
				Year:        r.SeasonYear,
				Episodes:    episodes,
				Cover:       cover,
				BlockReason: guard.blockReason(r.Id, episodes),
			})
		}

		JSONSuccess(w, http.StatusOK, out)
	}
}

// resolveTitle prefers the English title and falls back to romaji, the same rule as computeAnimeFields.
func resolveTitle(title anilist.Title) string {
	if title.English != nil && *title.English != "" {
		return *title.English
	}
	if title.Romaji != nil {
		return *title.Romaji
	}
	return ""
}
