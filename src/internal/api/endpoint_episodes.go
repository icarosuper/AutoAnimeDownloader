package api

import (
	"AutoAnimeDownloader/src/internal/logger"
	"net/http"
)

// @Summary      Get episodes list
// @Description  Returns a list of all downloaded episodes
// @Tags         episodes
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=[]files.EpisodeStruct}
// @Failure      405  {object}  SuccessResponse
// @Failure      500  {object}  SuccessResponse
// @Router       /episodes [get]
func handleEpisodes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}
		
		episodes, err := server.FileManager.LoadSavedEpisodes()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to load saved episodes")
			JSONInternalError(w, err)
			return
		}
		
		JSONSuccess(w, http.StatusOK, episodes)
	}
}

