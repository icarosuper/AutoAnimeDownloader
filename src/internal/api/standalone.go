package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/logger"
)

// loadStandaloneSet le o arquivo de avulsos como conjunto. Uma falha de leitura nao derruba o
// request: o pior efeito e a tela deixar de mostrar o chip e o detalhe do avulso voltar a 404,
// o que ja e o comportamento de quem nao tem avulso nenhum.
func loadStandaloneSet(fm FileManagerInterface) map[int]bool {
	ids, err := fm.LoadStandaloneAnimes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load standalone animes, continuing without them")
		return nil
	}
	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// resolveMediaList le um anime pelo media id com o fallback dos avulsos.
//
// anilist.GetAnimeInfo devolve (nil, nil) para quem nao esta na lista de conta nenhuma, e um
// avulso e exatamente esse caso — sem o fallback a tela de detalhe dele nao abre. O fallback e
// condicionado ao conjunto de avulsos de proposito: sem isso qualquer media id da AniList
// passaria a responder pelas rotas /animes/{id}/*, e o 404 de "esse anime nao e seu" sumiria.
func resolveMediaList(fm FileManagerInterface, id int, usernames []string, standalone map[int]bool) (*anilist.MediaList, error) {
	ml, err := anilist.GetAnimeInfo(id, usernames, anilist.PriorityCritical)
	// Servido do cache local conta como resposta: era esta tela que ficava inteira inacessivel
	// com a AniList fora do ar, e tudo o que ela faz alem de mostrar a lista de episodios
	// (bloquear, apagar, re-baixar, marcar como manual) ja e local. O banner do sistema avisa
	// que a AniList esta degradada; devolver 500 aqui so escondia o que ainda funcionava.
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, err
	}
	if ml != nil || !standalone[id] {
		// nil aqui, e nao err: neste ponto err e ErrFromCache ou nada, e o unico chamador
		// (handleAnimeEpisodes) transformaria qualquer erro em 500 — que e exatamente o que se
		// esta consertando. Quem avisa que a AniList caiu e o banner, por anilist.Health.
		return ml, nil
	}
	ml, err = anilist.GetMediaByID(id, anilist.PriorityCritical)
	if err != nil && !errors.Is(err, anilist.ErrFromCache) {
		return nil, err
	}
	return withStandaloneProgress(fm, ml), nil
}

// appendStandaloneEntries junta os avulsos as entradas vindas das listas da AniList, pulando os
// que ja aparecem nelas — a mesma regra (e a mesma razao) do append do daemon: a entrada real
// tem progresso e a sintetica nao.
//
// covered e atualizado no caminho para que refreshOrphanAnimes nao tente buscar de novo, por
// conta, um anime que nenhuma conta acompanha.
func appendStandaloneEntries(fm FileManagerInterface, entries []anilist.MediaList, standalone, covered map[int]bool) []anilist.MediaList {
	pending := make([]int, 0, len(standalone))
	for id := range standalone {
		if !covered[id] {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return entries
	}

	medias, err := anilist.GetMediaByIDs(pending, anilist.PriorityDisposable)
	switch {
	case errors.Is(err, anilist.ErrFromCache):
		logger.Logger.Warn().
			Msg("Serving standalone animes from the local cache: AniList is unreachable")
	case err != nil:
		logger.Logger.Warn().Err(err).
			Msg("Failed to fetch standalone animes from AniList; leaving the missing ones out of this response")
	}

	for _, id := range pending {
		ml, ok := medias[id]
		if !ok {
			continue // ja avisado pelo erro do lote
		}
		if ml == nil {
			logger.Logger.Warn().Int("media_id", id).
				Msg("AniList does not know this standalone media id; leaving it out of this response")
			continue
		}
		entries = append(entries, *withStandaloneProgress(fm, ml))
		covered[id] = true
	}
	return entries
}

// withStandaloneProgress — ver o gemeo em daemon/standalone.go. Sem ele a tela mostraria 0
// assistidos para um avulso cujo progresso o usuario acabou de gravar.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}

// handleStandaloneAnimes serves GET (list), POST (add one) and PUT (replace the whole file).
//
// The file is an int-list with no per-id metadata, so there is nothing to PATCH: the only sane
// bulk write is a wholesale replacement, which is what the CLI's `standalones import` needs to
// round-trip an export. GET and POST stay on the same path as before; PUT is new.
func handleStandaloneAnimes(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ids, err := server.FileManager.LoadStandaloneAnimes()
			if err != nil {
				logger.Logger.Error().Err(err).Msg("Failed to load standalone animes")
				JSONInternalError(w, err)
				return
			}
			JSONSuccess(w, http.StatusOK, ids)

		case http.MethodPost:
			handleStandaloneAnimeAdd(server)(w, r)

		case http.MethodPut:
			var ids []int
			if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
				JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Body must be a JSON array of media ids")
				return
			}
			for _, id := range ids {
				if id <= 0 {
					JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Media ids must be positive")
					return
				}
			}
			if err := server.FileManager.SaveStandaloneAnimes(ids); err != nil {
				logger.Logger.Error().Err(err).Msg("Failed to save standalone animes")
				JSONInternalError(w, err)
				return
			}
			JSONSuccess(w, http.StatusOK, map[string]any{"message": "Standalone animes replaced", "count": len(ids)})

		default:
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET, POST and PUT are allowed")
		}
	}
}
