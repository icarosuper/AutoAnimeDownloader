package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"testing"
	"time"
)

// O caso real: Mushoku Tensei S1 baixado como UM torrent da season inteira (1..23) sob a entrada
// do cour 1 (108465, 11 episodios). O cour 2 (127720) e outro media id, com numeracao propria
// 1..12, e sem posse por cobertura ele rebaixa a season inteira do Nyaa.
const (
	cour1ID    = 108465
	cour2ID    = 127720
	seasonHash = "aa11bb22cc33dd44ee55"
)

// cour2Anime monta a entrada do cour 2 com n episodios ja no ar, todos pendentes.
func cour2Anime(n int) anilist.MediaList {
	title := "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2"
	nodes := make([]anilist.AiringNode, n)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 2000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	return anilist.MediaList{
		Id:       cour2ID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:             cour2ID,
			Status:         anilist.MediaStatusFinished,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
}

// seasonPackOnDisk sao os 11 registros do cour 1, todos apontando para o pack que declara 1..23.
func seasonPackOnDisk() []files.EpisodeStruct {
	saved := make([]files.EpisodeStruct, 11)
	for i := range saved {
		saved[i] = files.EpisodeStruct{
			AnimeID:       cour1ID,
			AnimeName:     "Mushoku Tensei: Isekai Ittara Honki Dasu",
			EpisodeHash:   seasonHash,
			EpisodeNumber: i + 1,
			IsBatch:       true,
			BatchStart:    1,
			BatchEnd:      23,
			DownloadDate:  time.Now(),
		}
	}
	return saved
}

// countingSearcher falha o teste se qualquer busca for disparada, e devolve um pack que casaria
// com tudo — se a adocao nao funcionar, o teste acusa E o daemon teria baixado.
func countingSearcher(hit *bool) nyaaSearcher {
	return nyaaSearcher{
		searchAnime: func(_ anilist.Title, _ []string, _ []int) []nyaa.TorrentResult {
			*hit = true
			return []nyaa.TorrentResult{{Name: "[EMBER] Mushoku Tensei (2021) (Season 1) [Batch]", MagnetLink: "magnet:?xt=urn:btih:outro", IsBatch: true}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ int) []nyaa.TorrentResult {
			*hit = true
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool) []nyaa.TorrentResult { return nil },
	}
}

func coverageConfigs() *files.Config {
	return &files.Config{MaxEpisodesPerAnime: 12, EpisodeRetryLimit: 1}
}

// TestCoverageOwnership_SeasonPackCoversNextCour: o pack 1..23 baixado sob o cour 1 cobre, no
// eixo absoluto, os 12 episodios do cour 2 (offset 11 → abs 12..23). Nada vai para o Nyaa, nada e
// baixado, e os 12 registros novos apontam para o hash que ja esta em disco.
func TestCoverageOwnership_SeasonPackCoversNextCour(t *testing.T) {
	searched := false
	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(
		coverageConfigs(), backend, cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "[EMBER] Mushoku Tensei (2021) (Season 1) [Batch]", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched),
	)

	if searched {
		t.Error("nao deve buscar no Nyaa: o pack ja baixado cobre a janela inteira")
	}
	if len(backend.List()) > 0 {
		t.Errorf("nao deve adicionar torrent, mas o cliente tem %d", len(backend.List()))
	}
	if len(result.newEpisodes) != 12 {
		t.Fatalf("esperava 12 registros adotados, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.AnimeID != cour2ID || ep.EpisodeHash != seasonHash || !ep.IsBatch {
			t.Fatalf("registro adotado errado: %+v", ep)
		}
		// A faixa e CONVERTIDA para a regua do cour 2: o pack 1..23 do cour 1 (offset 0) e
		// -10..12 sob o cour 2 (offset 11). E exatamente o que assignBatches gravaria se a
		// busca reencontrasse o mesmo torrent (packAxis.localRange, decisions.md #79).
		if ep.BatchStart != -10 || ep.BatchEnd != 12 {
			t.Fatalf("faixa do registro adotado deveria ser -10-12, obteve %d-%d", ep.BatchStart, ep.BatchEnd)
		}
		if ep.EpisodeName != "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2 1-12" {
			t.Fatalf("nome exibido nao pode vazar a faixa negativa, obteve %q", ep.EpisodeName)
		}
	}
}

// A faixa gravada e lida de volta somando o offset do anime_id do PROPRIO registro. Se a adocao
// copiasse a faixa do dono sem converter, o registro do cour 2 (1..23 sob offset 11) seria lido
// como o absoluto 12..34 e um terceiro cour acharia cobertura para episodio que o pack nao tem —
// marcado como baixado, nunca buscado, arquivo inexistente.
func TestCoverageOwnership_AdoptedRangeDoesNotShiftForTheNextCour(t *testing.T) {
	const cour3ID = 146065
	index := map[int]anilist.Series{
		cour1ID: {Key: cour1ID, Offset: 0},
		cour2ID: {Key: cour1ID, Offset: 11},
		cour3ID: {Key: cour1ID, Offset: 23},
	}
	hashes := map[string]bool{seasonHash: true}
	pending := anilist.EpisodeList(cour2Anime(12), 1)

	adopted := adoptCoveredEpisodes(cour2Anime(12), "Cour 2", 12, pending, seasonPackOnDisk(), index, hashes)
	if len(adopted) != 12 {
		t.Fatalf("esperava os 12 do cour 2 adotados, obteve %d", len(adopted))
	}

	// O cour 3 comeca no absoluto 24, um passo depois do fim do pack.
	cour3 := cour2Anime(13)
	cour3.Id, cour3.Media.Id = cour3ID, cour3ID
	saved := append(seasonPackOnDisk(), adopted...)
	if leaked := adoptCoveredEpisodes(cour3, "Cour 3", 13, anilist.EpisodeList(cour3, 1), saved, index, hashes); len(leaked) != 0 {
		t.Fatalf("o pack termina no absoluto 23: o cour 3 nao pode adotar nada, adotou %d (ep %d)", len(leaked), leaked[0].EpisodeNumber)
	}
}

// TestCoverageOwnership_UncoveredEpisodesStillSearch: o cour 2 tem 14 episodios, o pack cobre ate
// o absoluto 23 (= local 12). Os dois que sobram continuam indo para a busca — adocao parcial nao
// pode virar "esta tudo resolvido".
func TestCoverageOwnership_UncoveredEpisodesStillSearch(t *testing.T) {
	searched := false
	configs := coverageConfigs()
	configs.MaxEpisodesPerAnime = 14

	result := processAnimeEpisodes(
		configs, torrents.NewFakeBackend(), cour2Anime(14),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched),
	)

	if !searched {
		t.Error("os episodios 13 e 14 nao sao cobertos pelo pack: a busca tem que rodar")
	}
	adopted := 0
	for _, ep := range result.newEpisodes {
		if ep.EpisodeHash == seasonHash {
			adopted++
		}
	}
	if adopted != 12 {
		t.Errorf("esperava 12 episodios adotados, obteve %d", adopted)
	}
}

// TestCoverageOwnership_OtherSeriesIsNotOwnership: sem a comparacao de Key, "absoluto 12" casaria
// com o pack de qualquer outro anime que tenha 12 na faixa.
func TestCoverageOwnership_OtherSeriesIsNotOwnership(t *testing.T) {
	searched := false

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: 16498, Offset: 11}, // outra serie, mesmo offset
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched),
	)

	if !searched {
		t.Error("pack de outra serie nao e posse: a busca tem que rodar")
	}
}

// TestCoverageOwnership_PackNotInSessionIsNotOwnership: registro apontando para um torrent que
// nao esta mais na sessao nao prova nada — adotar daria registro sem arquivo, invisivel para a
// redescida.
func TestCoverageOwnership_PackNotInSessionIsNotOwnership(t *testing.T) {
	searched := false

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		nil, // sessao vazia
		seasonPackOnDisk(),
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched),
	)

	if !searched {
		t.Error("torrent fora da sessao nao e posse: a busca tem que rodar")
	}
}

// TestCoverageOwnership_UnknownRangeIsNotOwnership: pack sem faixa no nome grava BatchStart == 0.
// Desconhecida nao e "cobre tudo" — mesmo teto de canRemoveTorrent (decisions.md #74).
func TestCoverageOwnership_UnknownRangeIsNotOwnership(t *testing.T) {
	searched := false

	saved := seasonPackOnDisk()
	for i := range saved {
		saved[i].BatchStart, saved[i].BatchEnd = 0, 0
	}

	processAnimeEpisodes(
		coverageConfigs(), torrents.NewFakeBackend(), cour2Anime(12),
		[]torrents.TorrentInfo{{Name: "pack", Hash: seasonHash}},
		saved,
		map[int]anilist.Series{
			cour1ID: {Key: cour1ID, Offset: 0},
			cour2ID: {Key: cour1ID, Offset: 11},
		},
		map[files.EpisodeKey]bool{}, countingSearcher(&searched),
	)

	if !searched {
		t.Error("pack sem faixa declarada nao e posse: a busca tem que rodar")
	}
}
