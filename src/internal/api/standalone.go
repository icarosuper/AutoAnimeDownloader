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
func resolveMediaList(id int, usernames []string, standalone map[int]bool) (*anilist.MediaList, error) {
	ml, err := anilist.GetAnimeInfo(id, usernames)
	if err != nil {
		return nil, err
	}
	if ml != nil || !standalone[id] {
		return ml, err
	}
	return anilist.GetMediaByID(id)
}

// appendStandaloneEntries junta os avulsos as entradas vindas das listas da AniList, pulando os
// que ja aparecem nelas — a mesma regra (e a mesma razao) do append do daemon: a entrada real
// tem progresso e a sintetica nao.
//
// covered e atualizado no caminho para que refreshOrphanAnimes nao tente buscar de novo, por
// conta, um anime que nenhuma conta acompanha.
func appendStandaloneEntries(entries []anilist.MediaList, standalone, covered map[int]bool) []anilist.MediaList {
	for id := range standalone {
		if covered[id] {
			continue
		}
		ml, err := anilist.GetMediaByID(id)
		if err != nil || ml == nil {
			logger.Logger.Warn().Err(err).Int("media_id", id).
				Msg("Failed to fetch a standalone anime from AniList; leaving it out of this response")
			continue
		}
		entries = append(entries, *ml)
		covered[id] = true
	}
	return entries
}
