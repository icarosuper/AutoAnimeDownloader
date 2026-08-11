package anilist

import "sort"

// EpisodeList devolve os episodios de um anime a partir de fromEpisode, e e a UNICA fonte de
// "quais episodios existem" no app.
//
// Nao da para usar media.airingSchedule cru para isso: a AniList guarda uma JANELA de ~500
// entradas de agenda por midia e descarta as antigas. No One Piece a pagina 1 comeca no episodio
// 1123 — os 1122 anteriores nao existem na API —, e num anime antigo/finalizado (Naruto, Shingeki
// no Kyojin) a agenda volta COMPLETAMENTE vazia. Enquanto a lista de episodios saia dai, esses
// animes simplesmente nao tinham episodios para baixar (ver decisions.md #52).
//
// Por isso a lista e SINTETIZADA de fromEpisode ate o ultimo episodio no ar, usando o no real da
// agenda quando ele existe (traz airingAt/timeUntilAiring de verdade) e um no fabricado quando
// nao. Os episodios ainda por vir vem apenas da agenda, que e quem sabe a data deles.
//
// Um no fabricado tem ID zero: depois da migracao da chave para (AnimeID, EpisodeNumber) nada
// identifica episodio por ID de agenda, entao ele nao precisa ser unico (ver decisions.md #52).
func EpisodeList(ml MediaList, fromEpisode int) []AiringNode {
	if fromEpisode < 1 {
		fromEpisode = 1
	}

	lastAired := lastAiredEpisode(ml)

	byEpisode := make(map[int]AiringNode, len(ml.Media.AiringSchedule.Nodes))
	for _, n := range ml.Media.AiringSchedule.Nodes {
		byEpisode[n.Episode] = n
	}

	result := make([]AiringNode, 0, max(lastAired-fromEpisode+1, 0)+len(ml.Media.AiringSchedule.Nodes))
	for ep := fromEpisode; ep <= lastAired; ep++ {
		if node, ok := byEpisode[ep]; ok {
			result = append(result, node)
			continue
		}
		// timeUntilAiring negativo = ja foi ao ar, que e a unica coisa que o resto do codigo
		// pergunta a um episodio passado. airingAt fica zerado: a AniList nao guarda mais a data.
		result = append(result, AiringNode{Episode: ep, TimeUntilAiring: -1})
	}

	// Os que ainda vao ao ar: a agenda, mais o nextAiringEpisode. Ele precisa entrar
	// explicitamente porque num anime de janela clipada a pagina 1 da agenda so tem episodios
	// PASSADOS (no One Piece ela vai do 1123 ao 1147, e o proximo e o 1174) — sem isso a tela
	// mostraria o anime sem nenhum episodio por vir.
	future := append([]AiringNode{}, ml.Media.AiringSchedule.Nodes...)
	if next := ml.Media.NextAiringEpisode; next != nil {
		future = append(future, *next)
	}
	seen := make(map[int]bool, len(future))
	for _, n := range future {
		if n.Episode > lastAired && n.Episode >= fromEpisode && !seen[n.Episode] {
			seen[n.Episode] = true
			result = append(result, n)
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Episode < result[j].Episode })
	return result
}

// lastAiredEpisode e o maior numero de episodio que ja foi ao ar, combinando as tres fontes que a
// AniList oferece — nenhuma delas confiavel sozinha: a agenda pode estar clipada ou vazia,
// nextAiringEpisode e nil em anime terminado, e media.episodes so vale quando o anime terminou
// (num RELEASING ele e a contagem PREVISTA da temporada inteira, nao o que ja passou).
// LastAiredEpisode e a versao exportada de lastAiredEpisode. Serve como medida de "tamanho da
// serie" fora do pacote — media.Episodes nao serve para isso: num RELEASING sem fim anunciado
// (One Piece) ele e nil, e e exatamente a serie longa que precisa ser reconhecida como tal.
func LastAiredEpisode(ml MediaList) int {
	return lastAiredEpisode(ml)
}

func lastAiredEpisode(ml MediaList) int {
	last := 0
	for _, n := range ml.Media.AiringSchedule.Nodes {
		if n.TimeUntilAiring <= 0 && n.Episode > last {
			last = n.Episode
		}
	}
	if next := ml.Media.NextAiringEpisode; next != nil && next.Episode-1 > last {
		last = next.Episode - 1
	}
	if ml.Media.Status == MediaStatusFinished && ml.Media.Episodes != nil && *ml.Media.Episodes > last {
		last = *ml.Media.Episodes
	}
	return last
}
