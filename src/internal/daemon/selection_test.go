package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// savedNonBatch monta o mapa de episodios salvos (1..n) sem marcacao de pack.
func savedNonBatch(animeID, n int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct) {
	set := make(map[files.EpisodeKey]bool, n)
	full := make(map[files.EpisodeKey]files.EpisodeStruct, n)
	for i := 1; i <= n; i++ {
		key := files.EpisodeKey{AnimeID: animeID, Episode: i}
		set[key] = true
		full[key] = files.EpisodeStruct{AnimeID: animeID, EpisodeNumber: i, EpisodeHash: "h"}
	}
	return set, full
}

// O bug que a ordem invertida evita: com o limite levantado por palpite, este conjunto vinha
// VAZIO — handleAlreadySavedEpisode nunca disparava com maxEpisodes = len(episodes)+1, e o
// tampao cortava episodesToDownload sem restaurar keysToDelete.
func TestSelectEpisodes_LimitProducesDeletions(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(sel.keysToDelete) != 38 {
		t.Errorf("esperava 38 episódios acima do limite em keysToDelete, obteve %d", len(sel.keysToDelete))
	}
	if len(sel.checked) != 50 {
		t.Errorf("todo episódio da lista deve ser checado, obteve %d", len(sel.checked))
	}
}

// Pureza: o contador de downloadedEpisodes e local, entao duas chamadas iguais devolvem o mesmo
// resultado. E o que torna a segunda selecao (limite levantado) de graca.
func TestSelectEpisodes_IsPure(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	first := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)
	second := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(first.toDownload) != len(second.toDownload) || len(first.keysToDelete) != len(second.keysToDelete) {
		t.Errorf("chamadas iguais devolveram resultados diferentes: %d/%d vs %d/%d",
			len(first.toDownload), len(first.keysToDelete), len(second.toDownload), len(second.keysToDelete))
	}
}

// Registro de pack estourando o limite NAO e apagado: o pack e a unidade, apagar um registro nao
// libera byte nenhum e o torrent continua no disco. E tambem nao baixa mais nada deste anime.
func TestHandleAlreadySavedEpisode_BatchIsImmuneToTheLimit(t *testing.T) {
	counter := 12

	if dl, del := handleAlreadySavedEpisode(12, &counter, true, true, "ep"); dl || del {
		t.Errorf("pack estourando o limite deve devolver (false, false), obteve (%v, %v)", dl, del)
	}
	if counter != 12 {
		t.Errorf("o contador não deve avançar no caminho imune, obteve %d", counter)
	}

	counter = 12
	if dl, del := handleAlreadySavedEpisode(12, &counter, true, false, "ep"); dl || !del {
		t.Errorf("episódio solto estourando o limite deve devolver (false, true), obteve (%v, %v)", dl, del)
	}
}

// Nao-churn: dois ciclos seguidos sobre o mesmo estado nao produzem delecao nenhuma.
func TestSelectEpisodes_BatchStateDoesNotChurn(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedMap := make(map[files.EpisodeKey]bool, 100)
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedMap[key] = true
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	for cycle := 1; cycle <= 2; cycle++ {
		sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"pack": true}, nil, nil)
		if len(sel.keysToDelete) != 0 {
			t.Fatalf("ciclo %d: pack salvo não deve gerar deleção, obteve %d", cycle, len(sel.keysToDelete))
		}
		if len(sel.toDownload) != 0 {
			t.Fatalf("ciclo %d: nada novo deve ser baixado, obteve %d", cycle, len(sel.toDownload))
		}
	}
}

// watched_episodes_to_keep quebraria o rodizio: manter 3 dos 100 de um pack nao guarda 3
// episodios, guarda o pack inteiro (allEpisodesInDeleteSet daria false e o torrent ficaria).
func TestBuildWatchedKeepSet_IgnoresBatchRecords(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100)
	if len(keepSet) != 0 {
		t.Errorf("esperava keep-set vazio para pack, obteve %d", len(keepSet))
	}

	// O par: registro sem IsBatch continua respeitando o keep.
	for key, ep := range savedFull {
		ep.IsBatch = false
		savedFull[key] = ep
	}
	if keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100); len(keepSet) != 3 {
		t.Errorf("episódio solto deve manter 3, obteve %d", len(keepSet))
	}
}

// Rodizio completo: com pack 1-100 salvo e progresso 100, os 100 registros entram no delete set,
// allEpisodesInDeleteSet passa e o torrent inteiro sai — e e isso que libera o espaco para o
// 101-200 no ciclo seguinte. Com progresso 50, nada sai (ha irmaos vivos) e nada novo entra.
func TestSelectEpisodes_PackRotation(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	build := func(progress int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct, anilist.MediaList) {
		a := anime
		a.Progress = progress
		savedMap := make(map[files.EpisodeKey]bool, 100)
		savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
		for i := 1; i <= 100; i++ {
			key := files.EpisodeKey{AnimeID: a.Media.Id, Episode: i}
			savedMap[key] = true
			savedFull[key] = files.EpisodeStruct{AnimeID: a.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
		}
		return savedMap, savedFull, a
	}

	// firstEpisodeToConsider precisa da lista real de salvos: e ela quem faz a janela recuar ate o
	// episodio 1 do pack. Sem isso a lista comecaria em progress+1 (101) e o pack watched nunca
	// seria checado, entao nunca entraria no delete set — o mesmo que processAnimeEpisodes faz.
	savedMap, savedFull, watchedAll := build(100)
	savedSlice := make([]files.EpisodeStruct, 0, len(savedFull))
	for _, ep := range savedFull {
		savedSlice = append(savedSlice, ep)
	}
	episodes := anilist.EpisodeList(watchedAll, firstEpisodeToConsider(watchedAll, savedSlice))
	keepSet := buildWatchedKeepSet(3, watchedAll.Media.Id, episodes, savedFull, watchedAll.Progress)
	sel := selectEpisodes(limitsConfig(), 12, watchedAll, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)

	deleteSet := make(map[files.EpisodeKey]bool, len(sel.keysToDelete))
	for _, k := range sel.keysToDelete {
		deleteSet[k] = true
	}
	group := make([]files.EpisodeStruct, 0, 100)
	for _, ep := range savedFull {
		group = append(group, ep)
	}
	if !allEpisodesInDeleteSet(group, deleteSet) {
		t.Errorf("com progresso 100 o grupo inteiro deve sair (%d de 100 no delete set)", len(deleteSet))
	}

	savedMap, savedFull, halfWatched := build(50)
	episodes = anilist.EpisodeList(halfWatched, firstEpisodeToConsider(halfWatched, nil))
	keepSet = buildWatchedKeepSet(3, halfWatched.Media.Id, episodes, savedFull, halfWatched.Progress)
	sel = selectEpisodes(limitsConfig(), 12, halfWatched, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)
	if len(sel.toDownload) != 0 {
		t.Errorf("com progresso 50 nada novo deve ser buscado, obteve %d", len(sel.toDownload))
	}
}

func TestEffectiveMaxAndWindowEnd(t *testing.T) {
	episodes := make([]anilist.AiringNode, 30)

	unlimited := &files.Config{MaxEpisodesPerAnime: 0}
	if got := effectiveMax(unlimited, episodes); got != 31 {
		t.Errorf("teto 0 é sem teto: esperava 31, obteve %d", got)
	}
	if got := windowEnd(unlimited, 5); got != math.MaxInt {
		t.Errorf("teto 0 abre a janela: esperava MaxInt, obteve %d", got)
	}

	limited := &files.Config{MaxEpisodesPerAnime: 12}
	if got := effectiveMax(limited, episodes); got != 12 {
		t.Errorf("esperava 12, obteve %d", got)
	}
	if got := windowEnd(limited, 95); got != 106 {
		t.Errorf("janela de 12 a partir do 95 termina em 106, obteve %d", got)
	}
}
