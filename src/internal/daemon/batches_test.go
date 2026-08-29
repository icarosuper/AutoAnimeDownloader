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

func names(results []nyaa.TorrentResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Name)
	}
	return out
}

func TestPickBatches_OnePackCoversTheWindow(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	if got := names(pickBatches(results, packAxis{}, 150, 161)); len(got) != 1 || got[0] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("primeiro pendente 150 deve escolher só o segundo pack, obteve %v", got)
	}
	if got := names(pickBatches(results, packAxis{}, 1, 12)); len(got) != 1 || got[0] != "[X] Anime 001-100 [1080p]" {
		t.Errorf("primeiro pendente 1 deve escolher só o primeiro pack, obteve %v", got)
	}
	if got := pickBatches(results, packAxis{}, 500, 511); len(got) != 0 {
		t.Errorf("sem pack que cubra, esperava vazio (o fluxo cai em episódio solto), obteve %v", names(got))
	}
}

// Janela cruzando a borda: 95..106 nao cabe num pack de 100, entao os DOIS entram, em ordem.
func TestPickBatches_WindowCrossingTheBoundaryPicksBoth(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	got := names(pickBatches(results, packAxis{}, 95, 106))
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

	if got := pickBatches(results, packAxis{}, 1, math.MaxInt); len(got) != 3 {
		t.Errorf("com o teto desligado esperava os três packs, obteve %v", names(got))
	}
}

// A ordem da lista E a ordem de qualidade (SortTorrentResults rodou antes), entao "o primeiro que
// cobre o cursor" e "o melhor que cobre o cursor".
func TestPickBatches_RespectsQualityOrder(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[Good] Anime 001-100 [1080p]", 1), pack("[Worse] Anime 001-100 [480p]", 2)}

	got := names(pickBatches(results, packAxis{}, 1, 12))
	if len(got) != 1 || got[0] != "[Good] Anime 001-100 [1080p]" {
		t.Errorf("esperava o primeiro da lista ordenada, obteve %v", got)
	}
}

// Terminacao: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente e nenhum pack e escolhido duas vezes.
func TestPickBatches_Terminates(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 50-60 [1080p]", 1)}

	got := pickBatches(results, packAxis{}, 55, math.MaxInt)
	if len(got) != 1 {
		t.Errorf("esperava exatamente 1 pack, obteve %d (%v)", len(got), names(got))
	}
}

// Pack sem faixa no nome vale como pack completo: cobre o resto da janela e encerra a busca.
func TestPickBatches_UnknownRangeCountsAsComplete(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1), pack("[X] Anime 001-100", 2)}

	got := pickBatches(results, packAxis{}, 1, math.MaxInt)
	if len(got) != 1 || got[0].Name != "[X] Anime Complete Batch" {
		t.Errorf("esperava só o pack completo, obteve %v", names(got))
	}
}

// Dois packs escolhidos: cada episódio recebe o magnet do pack da SUA faixa, e os overrideName
// são distintos (na tela de downloads dois packs do mesmo anime apareceriam com nomes idênticos).
func TestAssignBatches_EachEpisodeGetsItsOwnPack(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	first := pack("[X] Anime 001-100 [1080p]", 1)
	second := pack("[X] Anime 101-200 [1080p]", 2)

	episodes := anilist.EpisodeList(anime, 1)
	covered, magnets := assignBatches("Anime", packAxis{}, episodes, []nyaa.TorrentResult{first, second})

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

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1)})

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

	covered, magnets := assignBatches("Anime", packAxis{}, episodes, []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)})

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

	_, magnets := assignBatches("Anime", packAxis{}, episodes, []nyaa.TorrentResult{pack("[X] Anime 01-11 [1080p]", 1)})

	if magnets[6].batchStart != 1 || magnets[6].batchEnd != 11 {
		t.Errorf("esperava a faixa do pack (1-11), obteve %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
}

// Pack sem faixa no nome ("(Season 1+OVA) [Batch]") vale como pack COMPLETO — e por isso que
// coveringBatch o deixa cobrir qualquer episodio. Entao a faixa dele e a serie inteira, e nao o
// min/max dos pendentes: um pack completo baixado com 5 episodios ja assistidos aparecia "6-11".
func TestAssignBatches_UnknownRangeCoversTheWholeSeries(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 6)

	_, magnets := assignBatches("Anime", packAxis{totalEpisodes: 26}, episodes, []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)})

	if magnets[6].batchStart != 1 || magnets[6].batchEnd != 26 {
		t.Errorf("pack completo deve valer a série inteira (1-26), obteve %d-%d", magnets[6].batchStart, magnets[6].batchEnd)
	}
}

// Sem faixa no nome E sem total conhecido nao ha o que afirmar: zeros, e quem exibe cai no min/max.
func TestAssignBatches_UnknownRangeAndUnknownTotalLeavesZeroes(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusReleasing, false, "")
	episodes := anilist.EpisodeList(anime, 1)

	_, magnets := assignBatches("Anime", packAxis{}, episodes, []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)})

	if magnets[1].batchStart != 0 || magnets[1].batchEnd != 0 {
		t.Errorf("faixa e total desconhecidos devem ficar zerados, obteve %d-%d", magnets[1].batchStart, magnets[1].batchEnd)
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

// O caso real do Mushoku: o pack escolhido foi "[Anime Time] … (Season 1+OVA) … [Batch]", sem
// faixa no nome. O registro tem de sair com a serie inteira.
func TestProcessAnimeEpisodes_SavesTheWholeSeriesForARangelessPack(t *testing.T) {
	anime := animeWithEpisodes(11, anilist.MediaStatusFinished, true, "")
	anime.Progress = 5
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime (Season 1+OVA) [Batch]", MagnetLink: fakeMagnet(2)}}, nil, nil, nil)

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
