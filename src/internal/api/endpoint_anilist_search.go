package api

import (
	"net/http"
	"strings"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
)

// minSearchTermLength: abaixo disso a busca devolve lista vazia sem tocar a AniList. Uma ou
// duas letras casam com meio catalogo e o limite da AniList e de 30 req/min.
const minSearchTermLength = 3

type AniListSearchResult struct {
	Id     int    `json:"id" example:"21"`
	Title  string `json:"title" example:"One Piece"`
	Format string `json:"format" example:"TV"`
	Status string `json:"status" example:"RELEASING"`
	Year   int    `json:"year" example:"1999"`
	// Episodes e 0 quando a AniList nao sabe o total (o comum em anime ainda no ar).
	Episodes int    `json:"episodes" example:"24"`
	Cover    string `json:"cover,omitempty"`
	// BlockReason e "" quando o anime pode ser adicionado como avulso, senao um dos motivos de
	// standalone_guard.go. E UM CAMPO, nao quatro booleanos: os motivos sao mutuamente
	// exclusivos por precedencia e o card precisa de um rotulo so. Vem do mesmo blockReason que
	// o POST usa para decidir o 409 — e o que faz front e back concordarem por construcao.
	BlockReason string `json:"block_reason"`
}

// @Summary      Search animes on AniList
// @Description  Searches AniList by term and flags which results can be added as standalone animes
// @Tags         standalone
// @Produce      json
// @Param        q                   query  string  true   "Search term (fewer than 3 characters returns an empty list)"
// @Param        include_unreleased  query  bool    false  "Include animes that have not aired yet (NOT_YET_RELEASED). Default false"
// @Success      200  {object}  SuccessResponse{data=[]AniListSearchResult}
// @Failure      400  {object}  SuccessResponse
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /anilist/search [get]
func handleAniListSearch(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		term := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(term) < minSearchTermLength {
			// Lista vazia, nao 400: o frontend busca a cada tecla e um erro por tecla seria ruido.
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

// resolveTitle prefere o titulo em ingles e cai no romaji, a mesma regra de computeAnimeFields.
func resolveTitle(title anilist.Title) string {
	if title.English != nil && *title.English != "" {
		return *title.English
	}
	if title.Romaji != nil {
		return *title.Romaji
	}
	return ""
}
