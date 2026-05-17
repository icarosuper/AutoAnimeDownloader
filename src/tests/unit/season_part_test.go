package unit

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"testing"
)

// Helpers

func strPtr(s string) *string { return &s }

func makeTitle(english, romaji string) anilist.Title {
	t := anilist.Title{}
	if english != "" {
		t.English = strPtr(english)
	}
	if romaji != "" {
		t.Romaji = strPtr(romaji)
	}
	return t
}

func makeRelations(relType string, episodes int) anilist.MediaRelations {
	eps := episodes
	return anilist.MediaRelations{
		Edges: []anilist.MediaRelationEdge{
			{
				RelationType: relType,
				Node:         anilist.MediaRelationNode{Episodes: &eps},
			},
		},
	}
}

// ============================================
// ExtractAnimeSeasonPart
// ============================================

func TestExtractAnimeSeasonPart_SnKS3P2_FromRomaji(t *testing.T) {
	// Anilist id 104578: english = "Attack on Titan Season 3 Part 2"
	//                    romaji  = "Shingeki no Kyojin Season 3 Part 2"
	title := makeTitle("Attack on Titan Season 3 Part 2", "Shingeki no Kyojin Season 3 Part 2")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	if season == nil || *season != 3 {
		t.Errorf("expected season=3, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2, got %v", part)
	}
}

func TestExtractAnimeSeasonPart_KaguySa2_SeasonFromSynonyms(t *testing.T) {
	// Anilist id 112641: english = "Kaguya-sama: Love is War?" (sem season)
	//                    romaji  = "Kaguya-sama wa Kokurasetai?..." (sem season)
	//                    synonyms incluem "Kaguya-sama: Love is War Season 2"
	title := makeTitle("Kaguya-sama: Love is War?", "Kaguya-sama wa Kokurasetai?: Tensaitachi no Renai Zunousen")
	synonyms := []string{"Kaguya-sama: Love is War Season 2"}
	season, part := daemon.ExtractAnimeSeasonPart(title, synonyms)

	if season == nil || *season != 2 {
		t.Errorf("expected season=2 (from synonyms), got %v", season)
	}
	if part != nil {
		t.Errorf("expected part=nil, got %v", *part)
	}
}

func TestExtractAnimeSeasonPart_MushokuTenseiIIP2_PartFromRomaji(t *testing.T) {
	// Anilist id 166873: romaji = "Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2"
	title := makeTitle(
		"Mushoku Tensei: Jobless Reincarnation Season 2 Part 2",
		"Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2",
	)
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	// English dá season=2, romaji dá part=2
	if season == nil || *season != 2 {
		t.Errorf("expected season=2, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2, got %v", part)
	}
}

func TestExtractAnimeSeasonPart_NieRCour2_CourAsPart(t *testing.T) {
	// Anilist id 167420: english = "NieR:Automata Ver1.1a Cour 2"
	//                    romaji  = "NieR:Automata Ver1.1a 2nd Season"
	title := makeTitle("NieR:Automata Ver1.1a Cour 2", "NieR:Automata Ver1.1a 2nd Season")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	// "2nd Season" no romaji → season=2; "Cour 2" no english → part=2
	if season == nil || *season != 2 {
		t.Errorf("expected season=2, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2 (from Cour 2), got %v", part)
	}
}

func TestExtractAnimeSeasonPart_HatarakuS2P2_Inconsistency(t *testing.T) {
	// Anilist id 155168: english = "The Devil is a Part-Timer! Season 2 Part 2"
	//                    romaji  = "Hataraku Maou-sama!! 2nd Season" (sem part)
	title := makeTitle("The Devil is a Part-Timer! Season 2 Part 2", "Hataraku Maou-sama!! 2nd Season")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	// English dá season=2 e part=2
	if season == nil || *season != 2 {
		t.Errorf("expected season=2, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2, got %v", part)
	}
}

func TestExtractAnimeSeasonPart_SnKFinalSeason_NoSeasonNumber(t *testing.T) {
	// Anilist id 110277: "The Final Season" — sem número de season extraível
	title := makeTitle("Attack on Titan Final Season", "Shingeki no Kyojin: The Final Season")
	synonyms := []string{"Attack on Titan Season 4", "SnK 4"}
	season, part := daemon.ExtractAnimeSeasonPart(title, synonyms)

	// synonyms têm "Season 4" mas o próprio título não tem número
	// A implementação lê english primeiro: sem season. Romaji: sem season.
	// Synonyms: "Attack on Titan Season 4" → season=4
	if season == nil || *season != 4 {
		t.Errorf("expected season=4 (from synonyms), got %v", season)
	}
	if part != nil {
		t.Errorf("expected part=nil, got %v", *part)
	}
}

func TestExtractAnimeSeasonPart_NoSeasonOrPart(t *testing.T) {
	// Anime S1 simples, sem season/part no título
	title := makeTitle("Frieren: Beyond Journey's End", "Sousou no Frieren")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	if season != nil {
		t.Errorf("expected season=nil, got %v", *season)
	}
	if part != nil {
		t.Errorf("expected part=nil, got %v", *part)
	}
}

// ============================================
// ComputeEpisodeOffset
// ============================================

func TestComputeEpisodeOffset_SnKS3P2_Returns12(t *testing.T) {
	// Anilist id 104578: PREQUEL = SnK S3 (12 episódios)
	part2 := 2
	relations := makeRelations("PREQUEL", 12)
	offset := daemon.ComputeEpisodeOffset(relations, &part2)
	if offset != 12 {
		t.Errorf("expected offset=12, got %d", offset)
	}
}

func TestComputeEpisodeOffset_MushokuTenseiIIP2_Returns13(t *testing.T) {
	// Anilist id 166873: PREQUEL = MT II (13 episódios)
	part2 := 2
	relations := makeRelations("PREQUEL", 13)
	offset := daemon.ComputeEpisodeOffset(relations, &part2)
	if offset != 13 {
		t.Errorf("expected offset=13, got %d", offset)
	}
}

func TestComputeEpisodeOffset_NoPartGate_ReturnsZero(t *testing.T) {
	// Sem part >= 2, offset deve ser zero (previne Kaguya S2 receber offset errado)
	relations := makeRelations("PREQUEL", 12)

	// part=nil
	if got := daemon.ComputeEpisodeOffset(relations, nil); got != 0 {
		t.Errorf("expected 0 for nil part, got %d", got)
	}
	// part=1
	part1 := 1
	if got := daemon.ComputeEpisodeOffset(relations, &part1); got != 0 {
		t.Errorf("expected 0 for part=1, got %d", got)
	}
}

func TestComputeEpisodeOffset_NoPrequel_ReturnsZero(t *testing.T) {
	// Relações sem PREQUEL: sem offset
	part2 := 2
	relations := makeRelations("SEQUEL", 12)
	if got := daemon.ComputeEpisodeOffset(relations, &part2); got != 0 {
		t.Errorf("expected 0 without PREQUEL, got %d", got)
	}
}

func TestComputeEpisodeOffset_KaguySa2_NoPart_ReturnsZero(t *testing.T) {
	// Kaguya S2 (id 112641): part=nil → zero offset (gate obrigatório)
	relations := makeRelations("PREQUEL", 12)
	if got := daemon.ComputeEpisodeOffset(relations, nil); got != 0 {
		t.Errorf("expected 0 for Kaguya S2 (no part), got %d", got)
	}
}
