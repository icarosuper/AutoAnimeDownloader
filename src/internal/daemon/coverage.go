package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"fmt"
	"time"
)

// POSSE POR COBERTURA
//
// A unidade de posse do daemon e a chave files.EpisodeKey{AnimeID, Episode}, e um pack de season
// atravessa varios AnimeID: a AniList quebra cour e part em media ids separados, cada um com
// numeracao propria a partir de 1. Resultado sem isto aqui: o cour 2 nao tem registro nenhum,
// nao sabe que o arquivo dele ja esta em disco dentro do pack baixado sob o cour 1, e rebaixa.
//
// A conversao entre as duas reguas e o eixo absoluto da serie (anilist.Series, decisions.md #77):
// abs = Offset(AnimeID) + numero local. Comparar 1..23 com 1..12 sem converter e comparar reguas
// diferentes.

// resolveSeriesIndex resolve o eixo absoluto dos animes do passe MAIS o de todo anime_id que ja
// tem episodio em disco.
//
// Os de disco sao justamente os que faltam: o cour anterior, ja COMPLETED, saiu do universo do
// passe, e e o offset DELE que converte o BatchStart/BatchEnd do pack — gravado na numeracao
// local daquela entrada — para o eixo da serie. Sem essa semente a posse por cobertura nao
// enxergaria o pack que originou o bug.
//
// Erro nao aborta nada: GetSeriesIndex devolve o que conseguiu resolver, e id ausente do mapa so
// significa que aquele anime nao participa da adocao neste passe.
func resolveSeriesIndex(animeIDs []int, savedEpisodes []files.EpisodeStruct) map[int]anilist.Series {
	seed := make([]int, 0, len(animeIDs)+len(savedEpisodes))
	seen := make(map[int]bool, len(animeIDs)+len(savedEpisodes))
	add := func(id int) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		seed = append(seed, id)
	}
	for _, id := range animeIDs {
		add(id)
	}
	for _, ep := range savedEpisodes {
		add(ep.AnimeID)
	}

	index, err := anilist.GetSeriesIndex(seed, anilist.PriorityCritical)
	if err != nil {
		logger.Logger.Warn().Err(err).
			Int("seed", len(seed)).
			Int("resolved", len(index)).
			Msg("Failed to resolve part of the series index; coverage ownership is partial this pass")
	}
	return index
}

// adoptCoveredEpisodes adota os episodios pendentes cujo conteudo JA esta em disco: um pack
// baixado sob outra entrada da mesma serie cuja faixa declarada, no eixo absoluto, contem o
// episodio pedido.
//
// Adotar e gravar o registro apontando para o hash que ja existe — exatamente o que aconteceria
// se a busca reencontrasse o mesmo torrent, ja que Session.Add reusa o infohash. O ganho e nao
// passar pelo Nyaa nem rebaixar o que esta la; de quebra o guard de exclusao passa a enxergar os
// irmaos do outro cour (decisions.md #74) e o organize hardlinka os arquivos que nunca tiveram
// dono (organizeTorrent ja trata grupo parcialmente organizado, sem notificar de novo).
//
// Filme fica de fora: prequelOf so segue TV/TV_SHORT, entao um filme pos-season herda o offset da
// season e o "episodio 1" dele cairia dentro da faixa do pack dela. Cobertura por range nao
// distingue os dois.
func adoptCoveredEpisodes(
	anime anilist.MediaList,
	animeTitle string,
	totalEpisodes int,
	pending []anilist.AiringNode,
	savedEpisodes []files.EpisodeStruct,
	seriesIndex map[int]anilist.Series,
	torrentsHashSet map[string]bool,
) []files.EpisodeStruct {
	if len(pending) == 0 || isAnimeMovie(anime) {
		return nil
	}
	series, ok := seriesIndex[anime.Media.Id]
	if !ok {
		return nil
	}

	var adopted []files.EpisodeStruct
	for _, ep := range pending {
		owner := findCoveringPack(savedEpisodes, seriesIndex, torrentsHashSet, series, series.Offset+ep.Episode)
		if owner == nil {
			continue
		}

		logger.Logger.Info().
			Str("anime", animeTitle).
			Int("episode", ep.Episode).
			Int("absolute_episode", series.Offset+ep.Episode).
			Int("owner_anime_id", owner.AnimeID).
			Str("hash", owner.EpisodeHash).
			Msg("Adopting episode already covered by a downloaded pack")

		adopted = append(adopted, files.EpisodeStruct{
			AnimeID:            anime.Media.Id,
			AnimeTotalEpisodes: totalEpisodes,
			AnimeName:          animeTitle,
			EpisodeHash:        owner.EpisodeHash,
			EpisodeName:        fmt.Sprintf("%s %d-%d", animeTitle, owner.BatchStart, owner.BatchEnd),
			EpisodeNumber:      ep.Episode,
			IsBatch:            true,
			// A faixa e copiada como o dono a declara — ela e a do NOME do torrent, e o mesmo
			// hash pode ter registros de media ids em numeracoes locais diferentes. E o que
			// declaredSpan (episodes.go) espera: ele compara o span da uniao com a CONTAGEM de
			// registros, nunca com os numeros de episodio.
			BatchStart:   owner.BatchStart,
			BatchEnd:     owner.BatchEnd,
			DownloadDate: time.Now(),
		})
	}
	return adopted
}

// findCoveringPack devolve o registro de pack, em QUALQUER anime_id, cuja faixa declarada contem
// absEp no eixo absoluto — e cujo torrent ainda esta na sessao.
//
// Tres portas, todas obrigatorias: faixa declarada (pack sem faixa no nome grava BatchStart == 0,
// e desconhecida nao e "cobre tudo" — mesmo teto de canRemoveTorrent), torrent vivo (adotar um
// hash que nao existe mais deixaria o episodio com registro e sem arquivo, invisivel para a
// redescida) e MESMA serie (sem a comparacao de Key, "absoluto 12" casaria com o pack de qualquer
// outro anime).
func findCoveringPack(
	savedEpisodes []files.EpisodeStruct,
	seriesIndex map[int]anilist.Series,
	torrentsHashSet map[string]bool,
	series anilist.Series,
	absEp int,
) *files.EpisodeStruct {
	for i := range savedEpisodes {
		ep := &savedEpisodes[i]
		if ep.BatchStart <= 0 || ep.BatchEnd < ep.BatchStart {
			continue
		}
		if !episodeInTorrents(ep.EpisodeHash, torrentsHashSet) {
			continue
		}
		owner, ok := seriesIndex[ep.AnimeID]
		if !ok || owner.Key != series.Key {
			continue
		}
		if owner.Offset+ep.BatchStart <= absEp && absEp <= owner.Offset+ep.BatchEnd {
			return ep
		}
	}
	return nil
}

// dropAdopted tira da lista os episodios que a adocao ja resolveu.
//
// Roda nos DOIS pontos de selecao: selectEpisodes e pura e nao sabe da adocao, entao a segunda
// passada (limite levantado, caminho de pack) traria os adotados de volta e o daemon baixaria o
// que acabou de adotar.
func dropAdopted(episodes []anilist.AiringNode, adopted []files.EpisodeStruct) []anilist.AiringNode {
	if len(adopted) == 0 {
		return episodes
	}
	taken := make(map[int]bool, len(adopted))
	for _, ep := range adopted {
		taken[ep.EpisodeNumber] = true
	}
	remaining := make([]anilist.AiringNode, 0, len(episodes))
	for _, ep := range episodes {
		if !taken[ep.Episode] {
			remaining = append(remaining, ep)
		}
	}
	return remaining
}
