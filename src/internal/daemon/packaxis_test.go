package daemon

import (
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
)

// entryOf monta a entrada da AniList como ela chega ao passe: o titulo (de onde sai a part) e a
// relacao PREQUEL com a contagem de episodios dele (de onde sai a hipotese "continua pela season").
func entryOf(id int, title string, prequelEpisodes int) anilist.MediaList {
	media := anilist.Media{Id: id, Title: anilist.Title{Romaji: &title}}
	if prequelEpisodes > 0 {
		media.Relations = anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{{
			RelationType: "PREQUEL",
			Node:         anilist.MediaRelationNode{Episodes: &prequelEpisodes, Format: anilist.MediaFormatTV},
		}}}
	}
	return anilist.MediaList{Id: id, Media: media}
}

// As QUATRO convencoes de numeracao de pack medidas em sources.md ("Granularidade e numeracao dos
// packs"), cada uma sob a entrada real em que ela aparece. O que se cobra e a faixa CONVERTIDA
// para a numeracao local da entrada: e ela que vai para o registro e que a posse por cobertura
// (decisions.md #78) reconverte para o eixo absoluto.
func TestPackAxis_TheFourNumberingConventions(t *testing.T) {
	cases := []struct {
		name       string
		anime      anilist.MediaList
		offset     int // eixo absoluto da serie (decisions.md #77)
		total      int // episodios da ENTRADA
		pack       string
		start, end int
		convention string
	}{
		{
			name:       "reinicia por season",
			anime:      entryOf(146065, "Mushoku Tensei II: Isekai Ittara Honki Dasu", 12),
			offset:     23,
			total:      13,
			pack:       "[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu - 00 ~ 12 [BATCH]",
			start:      0,
			end:        12,
			convention: "numeracao da propria entrada, comecando em zero",
		},
		{
			name:       "reinicia por part",
			anime:      entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 13),
			offset:     36,
			total:      12,
			pack:       "[EMBER] Mushoku Tensei II (Season 2 | Part 2) 01 ~ 12 (Batch)",
			start:      1,
			end:        12,
			convention: "numeracao da propria entrada",
		},
		{
			name:       "continua pela season",
			anime:      entryOf(127720, "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2", 11),
			offset:     11,
			total:      12,
			pack:       "[Judas] Mushoku Tensei - Isekai Ittara Honki Dasu - 01 ~ 23 [BD 1080p][Batch]",
			start:      -10,
			end:        12,
			convention: "pack da season inteira: comeca 10 episodios ANTES do ep 1 desta entrada",
		},
		{
			name:       "absoluta pela serie",
			anime:      entryOf(131681, "Shingeki no Kyojin: The Final Season Part 2", 16),
			offset:     75,
			total:      12,
			pack:       "[SubsPlease] Shingeki no Kyojin - 76-87 (1080p) [Batch]",
			start:      1,
			end:        12,
			convention: "numeracao absoluta da franquia",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := map[int]anilist.Series{tc.anime.Media.Id: {Key: 1, Offset: tc.offset}}
			axis := newPackAxis(tc.anime, index, tc.total)

			picked := pickBatches(newPackSet([]nyaa.TorrentResult{pack(tc.pack, 1)}, nil), axis, 1, tc.total)
			if picked.len() != 1 {
				t.Fatalf("%s: o pack deveria cobrir a janela pendente, obteve %d packs", tc.convention, picked.len())
			}

			_, info := picked.covering(axis, 1)
			if info.StartEpisode != tc.start || info.EndEpisode != tc.end {
				t.Fatalf("%s: esperava faixa local %d-%d, obteve %d-%d",
					tc.convention, tc.start, tc.end, info.StartEpisode, info.EndEpisode)
			}
		})
	}
}

// O desempate por contagem: dois packs cobrem o episodio 1 sob a MESMA entrada de 12 episodios, e
// so o span diz qual e qual. Sem ele o delta 0 casaria os dois e o pack de season seria gravado
// como se comecasse no episodio 1 desta entrada.
func TestPackAxis_SpanDecidesBetweenLocalAndSeasonPack(t *testing.T) {
	anime := entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 13)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 36}}, 12)

	local := nyaa.ExtractBatchInfo("[EMBER] Mushoku Tensei II (Season 2 | Part 2) 01 ~ 12 (Batch)")
	if got, ok := axis.localRange(local, 1); !ok || got.StartEpisode != 1 || got.EndEpisode != 12 {
		t.Fatalf("pack de 12 episodios e local: esperava 1-12, obteve %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}

	season := nyaa.ExtractBatchInfo("[Diddy] Mushoku Tensei II - 01 ~ 25 [BD]")
	if got, ok := axis.localRange(season, 1); !ok || got.StartEpisode != -12 || got.EndEpisode != 12 {
		t.Fatalf("pack de 25 episodios e da season: esperava -12-12, obteve %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}
}

// A faixa lida da lista de arquivos entra pelo MESMO eixo da faixa lida do nome: os grupos usam
// as mesmas quatro convencoes nos arquivos que no titulo. Este e o caso medido do EMBER
// "(Season 2 | Part 2) … (Batch)" — nome sem faixa, arquivos 13..24, entrada Part 2 de 12
// episodios. Sem a conversao, 13..24 nao cobriria episodio nenhum desta entrada.
func TestPackSet_DetailRangeGoesThroughTheAxis(t *testing.T) {
	anime := entryOf(166873, "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 12)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 12}}, 12)

	set := newPackSet([]nyaa.TorrentResult{pack("[EMBER] Mushoku Tensei II (Season 2 | Part 2) (Batch)", 1)},
		func(nyaa.TorrentResult) (nyaa.BatchInfo, bool) {
			return nyaa.BatchInfo{StartEpisode: 13, EndEpisode: 24}, true
		})

	got, info := set.covering(axis, 1)
	if got == nil {
		t.Fatal("o pack cobre o episódio 1 desta entrada pelo eixo da season")
	}
	if info.StartEpisode != 1 || info.EndEpisode != 12 {
		t.Fatalf("esperava a faixa convertida 1-12, obteve %d-%d", info.StartEpisode, info.EndEpisode)
	}
}

// Na duvida, delta 0: sem informacao de serie nenhuma hipotese alternativa existe, e o pack e lido
// na numeracao da entrada — o comportamento de sempre, que e o que o zero-value entrega.
func TestPackAxis_ZeroValueKeepsRelativeNumbering(t *testing.T) {
	info := nyaa.ExtractBatchInfo("[X] Anime 01-12 [Batch]")
	got, ok := packAxis{}.localRange(info, 1)
	if !ok || got.StartEpisode != 1 || got.EndEpisode != 12 {
		t.Fatalf("esperava 1-12 na numeracao relativa, obteve %d-%d (ok=%v)", got.StartEpisode, got.EndEpisode, ok)
	}
}

// Pack que nao cobre o episodio em hipotese nenhuma nao entra: a busca cai em episodio solto, e
// nao num pack que nao tem o que se quer.
func TestPackAxis_NoHypothesisCoversRejectsThePack(t *testing.T) {
	anime := entryOf(166873, "Anime Part 2", 13)
	axis := newPackAxis(anime, map[int]anilist.Series{166873: {Key: 1, Offset: 36}}, 12)

	if picked := pickBatches(newPackSet([]nyaa.TorrentResult{pack("[X] Anime 200-212 [Batch]", 1)}, nil), axis, 1, 12); picked.len() != 0 {
		t.Fatalf("esperava nenhum pack, obteve %v", batchNames(picked))
	}
}

// A faixa gravada e a convertida (pode comecar em zero ou abaixo), mas o nome exibido corta em 1:
// "Anime -10-12" nao diz nada na tela de downloads.
func TestAssignBatches_RecordsConvertedRangeAndClampsTheDisplayName(t *testing.T) {
	anime := entryOf(127720, "Mushoku Tensei: Isekai Ittara Honki Dasu Part 2", 11)
	axis := newPackAxis(anime, map[int]anilist.Series{127720: {Key: 1, Offset: 11}}, 12)

	episodes := []anilist.AiringNode{{Episode: 1}, {Episode: 2}}
	batch := pack("[Judas] Mushoku Tensei - 01 ~ 23 [BD 1080p][Batch]", 1)

	covered, magnets := assignBatches("Anime", axis, episodes, newPackSet([]nyaa.TorrentResult{batch}, nil))
	if len(covered) != 2 {
		t.Fatalf("esperava os dois episodios cobertos, obteve %d", len(covered))
	}
	got := magnets[1]
	if got.batchStart != -10 || got.batchEnd != 12 {
		t.Fatalf("esperava faixa gravada -10-12, obteve %d-%d", got.batchStart, got.batchEnd)
	}
	if got.overrideName != "Anime 1-12" {
		t.Fatalf("esperava nome exibido \"Anime 1-12\", obteve %q", got.overrideName)
	}
}

// A faixa convertida chega ao guard de exclusao e a posse por cobertura: o sentinela de
// "desconhecida" e o FIM, entao um pack de season gravado sob um cour posterior (comeco negativo)
// continua declarando os 23 episodios de conteudo que tem — que e o que impede a poda de apagar o
// cour que ainda nao foi baixado (decisions.md #74).
func TestDeclaredSpan_ConvertedRangeStartingBeforeEpisodeOne(t *testing.T) {
	group := []files.EpisodeStruct{
		{AnimeID: 127720, EpisodeNumber: 1, BatchStart: -10, BatchEnd: 12},
		{AnimeID: 127720, EpisodeNumber: 2, BatchStart: -10, BatchEnd: 12},
	}

	if got := declaredSpan(group); got != 23 {
		t.Fatalf("esperava span 23 (o pack inteiro), obteve %d", got)
	}
	if !hasDeclaredRange(group[0]) {
		t.Fatal("comeco negativo e faixa declarada, nao desconhecida")
	}
	if hasDeclaredRange(files.EpisodeStruct{BatchStart: 0, BatchEnd: 0}) {
		t.Fatal("pack sem faixa no nome continua sendo desconhecida")
	}
}
