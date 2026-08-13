package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/nyaa"
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

	if got := names(pickBatches(results, 150, 161)); len(got) != 1 || got[0] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("primeiro pendente 150 deve escolher só o segundo pack, obteve %v", got)
	}
	if got := names(pickBatches(results, 1, 12)); len(got) != 1 || got[0] != "[X] Anime 001-100 [1080p]" {
		t.Errorf("primeiro pendente 1 deve escolher só o primeiro pack, obteve %v", got)
	}
	if got := pickBatches(results, 500, 511); len(got) != 0 {
		t.Errorf("sem pack que cubra, esperava vazio (o fluxo cai em episódio solto), obteve %v", names(got))
	}
}

// Janela cruzando a borda: 95..106 nao cabe num pack de 100, entao os DOIS entram, em ordem.
func TestPickBatches_WindowCrossingTheBoundaryPicksBoth(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	got := names(pickBatches(results, 95, 106))
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

	if got := pickBatches(results, 1, math.MaxInt); len(got) != 3 {
		t.Errorf("com o teto desligado esperava os três packs, obteve %v", names(got))
	}
}

// A ordem da lista E a ordem de qualidade (SortTorrentResults rodou antes), entao "o primeiro que
// cobre o cursor" e "o melhor que cobre o cursor".
func TestPickBatches_RespectsQualityOrder(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[Good] Anime 001-100 [1080p]", 1), pack("[Worse] Anime 001-100 [480p]", 2)}

	got := names(pickBatches(results, 1, 12))
	if len(got) != 1 || got[0] != "[Good] Anime 001-100 [1080p]" {
		t.Errorf("esperava o primeiro da lista ordenada, obteve %v", got)
	}
}

// Terminacao: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente e nenhum pack e escolhido duas vezes.
func TestPickBatches_Terminates(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 50-60 [1080p]", 1)}

	got := pickBatches(results, 55, math.MaxInt)
	if len(got) != 1 {
		t.Errorf("esperava exatamente 1 pack, obteve %d (%v)", len(got), names(got))
	}
}

// Pack sem faixa no nome vale como pack completo: cobre o resto da janela e encerra a busca.
func TestPickBatches_UnknownRangeCountsAsComplete(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1), pack("[X] Anime 001-100", 2)}

	got := pickBatches(results, 1, math.MaxInt)
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
	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{first, second})

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

	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1)})

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

	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)})

	if len(covered) != 26 {
		t.Fatalf("esperava 26 episódios cobertos, obteve %d", len(covered))
	}
	if magnets[1].overrideName != "Anime" {
		t.Errorf("faixa desconhecida deve usar o título do anime, obteve %q", magnets[1].overrideName)
	}
}
