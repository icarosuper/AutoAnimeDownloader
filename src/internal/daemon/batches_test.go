package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func pack(name string, n int) nyaa.TorrentResult {
	return nyaa.TorrentResult{Name: name, MagnetLink: fakeMagnet(n), IsBatch: true}
}

func TestPickBatches_OnePackCoversTheWindow(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	if got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 150, 161)); len(got) != 1 || got[0] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("primeiro pendente 150 deve escolher só o segundo pack, obteve %v", got)
	}
	if got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 1, 12)); len(got) != 1 || got[0] != "[X] Anime 001-100 [1080p]" {
		t.Errorf("primeiro pendente 1 deve escolher só o primeiro pack, obteve %v", got)
	}
	if got := pickBatches(newPackSet(results, nil), packAxis{}, 500, 511); got.len() != 0 {
		t.Errorf("sem pack que cubra, esperava vazio (o fluxo cai em episódio solto), obteve %v", batchNames(got))
	}
}

// Janela cruzando a borda: 95..106 nao cabe num pack de 100, entao os DOIS entram, em ordem.
func TestPickBatches_WindowCrossingTheBoundaryPicksBoth(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 95, 106))
	if len(got) != 2 || got[0] != "[X] Anime 001-100 [1080p]" || got[1] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("esperava os dois packs em ordem, obteve %v", got)
	}
}

func TestPickBatches_CeilingOffCoversEverything(t *testing.T) {
	results := []nyaa.TorrentResult{
		pack("[X] Anime 1-100 [1080p]", 1),
		pack("[X] Anime 101-200 [1080p]", 2),
		pack("[X] Anime 201-300 [1080p]", 3),
	}

	if got := pickBatches(newPackSet(results, nil), packAxis{}, 1, math.MaxInt); got.len() != 3 {
		t.Errorf("com o teto desligado esperava os três packs, obteve %v", batchNames(got))
	}
}

// A ordem da lista E a ordem de qualidade (SortTorrentResults rodou antes), entao "o primeiro que
// cobre o cursor" e "o melhor que cobre o cursor".
func TestPickBatches_RespectsQualityOrder(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[Good] Anime 001-100 [1080p]", 1), pack("[Worse] Anime 001-100 [480p]", 2)}

	got := batchNames(pickBatches(newPackSet(results, nil), packAxis{}, 1, 12))
	if len(got) != 1 || got[0] != "[Good] Anime 001-100 [1080p]" {
		t.Errorf("esperava o primeiro da lista ordenada, obteve %v", got)
	}
}

// Terminacao: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente e nenhum pack e escolhido duas vezes.
func TestPickBatches_Terminates(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 50-60 [1080p]", 1)}

	got := pickBatches(newPackSet(results, nil), packAxis{}, 55, math.MaxInt)
	if got.len() != 1 {
		t.Errorf("esperava exatamente 1 pack, obteve %d (%v)", got.len(), batchNames(got))
	}
}

// Pack sem faixa no nome vale como pack completo: cobre o resto da janela e encerra a busca.
func TestPickBatches_UnknownRangeCountsAsComplete(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1), pack("[X] Anime 001-100", 2)}

	got := pickBatches(newPackSet(results, nil), packAxis{}, 1, math.MaxInt)
	if got.len() != 1 || got.items[0].torrent.Name != "[X] Anime Complete Batch" {
		t.Errorf("esperava só o pack completo, obteve %v", batchNames(got))
	}
}

// Dois packs escolhidos: cada episódio recebe o magnet do pack da SUA faixa, e os overrideName
// são distintos (na tela de downloads dois packs do mesmo anime apareceriam com nomes idênticos).
func TestAssignBatches_EachEpisodeGetsItsOwnPack(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	first := pack("[X] Anime 001-100 [1080p]", 1)
	second := pack("[X] Anime 101-200 [1080p]", 2)

	episodes := anilist.EpisodeList(anime, 1)
	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{first, second}, nil))

	if len(covered) != 200 {
		t.Fatalf("esperava 200 episódios cobertos, obteve %d", len(covered))
	}
	if magnets[50].magnets[0] != first.MagnetLink || magnets[150].magnets[0] != second.MagnetLink {
		t.Error("cada episódio deve receber o magnet do pack da sua faixa")
	}
	if magnets[50].overrideName == magnets[150].overrideName {
		t.Errorf("os overrideName devem ser distintos, ambos são %q", magnets[50].overrideName)
	}
	if magnets[50].overrideName != "Anime 1-100" {
		t.Errorf("esperava \"Anime 1-100\", obteve %q", magnets[50].overrideName)
	}
	if !magnets[50].skipSubfolder {
		t.Error("episódio de pack precisa de skipSubfolder")
	}
}

// Cobertura parcial: os episódios de fora NÃO caem em episódio solto (em One Piece isso dispararia
// ~1000 notificações ReasonNotFound por passada). O corte é de prefixo.
func TestAssignBatches_TruncatesUncoveredEpisodes(t *testing.T) {
	anime := animeWithEpisodes(500, anilist.MediaStatusReleasing, false, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1)}, nil))

	if len(covered) != 100 {
		t.Fatalf("esperava 100 episódios cobertos, obteve %d", len(covered))
	}
	if covered[99].Episode != 100 {
		t.Errorf("o corte deve preservar o prefixo, último coberto = %d", covered[99].Episode)
	}
	if _, ok := magnets[101]; ok {
		t.Error("episódio fora da cobertura não pode receber magnet")
	}
}

// Pack sem faixa no nome cobre tudo: nenhum truncamento e o nome cai no título do anime.
func TestAssignBatches_UnknownRangeUsesAnimeTitle(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)}, nil))

	if len(covered) != 26 {
		t.Fatalf("esperava 26 episódios cobertos, obteve %d", len(covered))
	}
	if magnets[1].overrideName != "Anime" {
		t.Errorf("faixa desconhecida deve usar o título do anime, obteve %q", magnets[1].overrideName)
	}
}

// A faixa REAL do pack (a que está no nome do torrent) tem de sobreviver ao registro: a tela de
// detalhe montava "Episódios 6–11" para um pack 01-11 baixado com 5 episódios já assistidos,
// porque reconstruía a faixa pelo min/max dos episódios SALVOS — que são só os pendentes.
func TestAssignBatches_CarriesTheRealRange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 6)

	_, magnets := assignBatches("Anime", packAxis{}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime 01-11 [1080p]", 1)}, nil))

	if magnets[6].batchStart != 1 || magnets[6].batchEnd != 11 {
		t.Errorf("esperava a faixa do pack (1-11), obteve %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
}

// Faixa que nem o nome nem a lista de arquivos resolveram grava ZERO, nao 1..total. O pack ainda
// e usado (covering o trata como completo), mas o registro nao afirma uma cobertura que ninguem
// conferiu — 1..total alimentava a posse por cobertura com um palpite (decisions.md #84).
func TestAssignBatches_UnresolvedRangeLeavesZeroes(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 6)

	_, magnets := assignBatches("Anime", packAxis{totalEpisodes: 26}, episodes, newPackSet([]nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)}, nil))

	if magnets[6].batchStart != 0 || magnets[6].batchEnd != 0 {
		t.Errorf("faixa nao resolvida deve ficar zerada, obteve %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
	if magnets[6].overrideName != "Anime" {
		t.Errorf("sem faixa o nome exibido e so o do anime, obteve %q", magnets[6].overrideName)
	}
}

// O caso do bug, ponta a ponta: 11 episódios, 5 assistidos, pack 01-11. Só os pendentes viram
// registro (6..11), mas TODOS carregam a faixa do pack.
func TestProcessAnimeEpisodes_SavesThePackRange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	anime.Progress = 5
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-11 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("esperava os 6 episódios pendentes registrados, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.BatchStart != 1 || ep.BatchEnd != 11 {
			t.Errorf("episódio %d: esperava faixa 1-11, obteve %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// Pack sem faixa no nome e sem detalhe legivel: o download acontece igual (o Nyaa pode estar
// fora do ar, e recusar o pack zeraria os packs de todo mundo), mas a faixa gravada fica
// desconhecida em vez de 1..total.
func TestProcessAnimeEpisodes_UnresolvedRangeLeavesTheRecordWithoutARange(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	anime.Progress = 5
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime (Season 1+OVA) [Batch]", MagnetLink: fakeMagnet(2)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("esperava os 6 episódios pendentes registrados, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.BatchStart != 0 || ep.BatchEnd != 0 {
			t.Errorf("episódio %d: esperava faixa desconhecida, obteve %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// O BUG, ponta a ponta: o pack nao declara faixa no nome e traz so os episodios 1..6 de uma
// entrada de 11. Antes, "faixa desconhecida" valia como "cobre tudo" e os 11 viravam registro
// apontando para o hash do pack — 5 episodios marcados como baixados sem arquivo nenhum, e nada
// reconcilia registro com disco depois. Com a lista de arquivos, so os 6 que existem viram
// registro; o resto volta na proxima passada.
func TestProcessAnimeEpisodes_DetailPageRangeStopsPhantomEpisodes(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime [Batch]", MagnetLink: fakeMagnet(3), DetailURL: "/view/1"}}, nil, nil, nil)
	searcher.packRange = func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		return nyaa.BatchInfo{StartEpisode: 1, EndEpisode: 6}, true
	}

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 6 {
		t.Fatalf("esperava registro só para os 6 episódios que o pack tem, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.EpisodeNumber > 6 {
			t.Errorf("episódio %d não está no pack e não podia virar registro", ep.EpisodeNumber)
		}
		if ep.BatchStart != 1 || ep.BatchEnd != 6 {
			t.Errorf("episódio %d: esperava a faixa lida dos arquivos (1-6), obteve %d-%d", ep.EpisodeNumber, ep.BatchStart, ep.BatchEnd)
		}
	}
}

// A pagina de detalhe so e buscada para pack cujo NOME nao da a faixa, e so ate o orcamento.
func TestPackSet_FetchesDetailOnlyForRangelessPacksWithinBudget(t *testing.T) {
	fetches := 0
	detail := func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		fetches++
		return nyaa.BatchInfo{}, false
	}

	// O primeiro pack ja declara a faixa e cobre a janela: nenhuma requisicao extra sai.
	pickBatches(newPackSet([]nyaa.TorrentResult{pack("[X] Anime 01-12 [1080p]", 1), pack("[X] Anime [Batch]", 2)}, detail), packAxis{}, 1, 12)
	if fetches != 0 {
		t.Errorf("pack com faixa no nome não devia custar requisição, obteve %d", fetches)
	}

	// Nenhum pack cobre a janela pela faixa do nome: os sem faixa sao consultados, ate o teto.
	rangeless := make([]nyaa.TorrentResult, 0, maxPackDetailFetches+2)
	for i := 0; i < maxPackDetailFetches+2; i++ {
		rangeless = append(rangeless, pack("[X] Anime [Batch]", i+10))
	}
	fetches = 0
	set := newPackSet(rangeless, detail)
	for i := range set.items {
		set.rangeOf(i)
	}
	if fetches != maxPackDetailFetches {
		t.Errorf("esperava no máximo %d requisições de detalhe, obteve %d", maxPackDetailFetches, fetches)
	}
}

// Detalhe que nao responde nao e tentado de novo no mesmo passe: assignBatches varre os packs uma
// vez por episodio, e sem memoria isso viraria uma requisicao por episodio.
func TestPackSet_RemembersAFailedDetailLookup(t *testing.T) {
	fetches := 0
	set := newPackSet([]nyaa.TorrentResult{pack("[X] Anime [Batch]", 1)}, func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
		fetches++
		return nyaa.BatchInfo{}, false
	})

	for i := 0; i < 5; i++ {
		set.rangeOf(0)
	}
	if fetches != 1 {
		t.Errorf("esperava uma única tentativa, obteve %d", fetches)
	}
}
