package api

import (
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
	ml, err := anilist.GetAnimeInfo(id, usernames)
	if err != nil {
		return nil, err
	}
	if ml != nil || !standalone[id] {
		return ml, err
	}
	ml, err = anilist.GetMediaByID(id)
	if err != nil {
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

	medias, err := anilist.GetMediaByIDs(pending)
	if err != nil {
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
