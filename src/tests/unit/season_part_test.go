package unit

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/nyaa"
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
	return makeRelationsFmt(relType, episodes, anilist.MediaFormatTV)
}

func makeRelationsFmt(relType string, episodes int, format anilist.MediaFormat) anilist.MediaRelations {
	eps := episodes
	return anilist.MediaRelations{
		Edges: []anilist.MediaRelationEdge{
			{
				RelationType: relType,
				Node:         anilist.MediaRelationNode{Episodes: &eps, Format: format},
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
	// Anilist id 112641: english = "Kaguya-sama: Love is War?" (no season)
	//                    romaji  = "Kaguya-sama wa Kokurasetai?..." (no season)
	//                    synonyms include "Kaguya-sama: Love is War Season 2"
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

	// English gives season=2, romaji gives part=2
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

	// "2nd Season" in romaji → season=2; "Cour 2" in english → part=2
	if season == nil || *season != 2 {
		t.Errorf("expected season=2, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2 (from Cour 2), got %v", part)
	}
}

func TestExtractAnimeSeasonPart_HatarakuS2P2_Inconsistency(t *testing.T) {
	// Anilist id 155168: english = "The Devil is a Part-Timer! Season 2 Part 2"
	//                    romaji  = "Hataraku Maou-sama!! 2nd Season" (no part)
	title := makeTitle("The Devil is a Part-Timer! Season 2 Part 2", "Hataraku Maou-sama!! 2nd Season")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	// English gives season=2 and part=2
	if season == nil || *season != 2 {
		t.Errorf("expected season=2, got %v", season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2, got %v", part)
	}
}

func TestExtractAnimeSeasonPart_SnKFinalSeason_NoSeasonNumber(t *testing.T) {
	// Anilist id 110277: "The Final Season" — no extractable season number
	title := makeTitle("Attack on Titan Final Season", "Shingeki no Kyojin: The Final Season")
	synonyms := []string{"Attack on Titan Season 4", "SnK 4"}
	season, part := daemon.ExtractAnimeSeasonPart(title, synonyms)

	// synonyms have "Season 4" but the title itself has no number
	// The implementation reads english first: no season. Romaji: no season.
	// Synonyms: "Attack on Titan Season 4" → season=4
	if season == nil || *season != 4 {
		t.Errorf("expected season=4 (from synonyms), got %v", season)
	}
	if part != nil {
		t.Errorf("expected part=nil, got %v", *part)
	}
}

func TestExtractAnimeSeasonPart_RomanNumeralOnly_SeasonFromRomaji(t *testing.T) {
	// Anilist id 194829: english = "From Old Country Bumpkin to Master Swordsman II"
	//                    romaji  = "Katainaka no Ossan, Kensei ni Naru II"
	// Neither english nor romaji say "Season 2"/"S2" — only the roman numeral "II".
	title := makeTitle("From Old Country Bumpkin to Master Swordsman II", "Katainaka no Ossan, Kensei ni Naru II")
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)

	if season == nil || *season != 2 {
		t.Errorf("expected season=2 (from roman numeral), got %v", season)
	}
	if part != nil {
		t.Errorf("expected part=nil, got %v", *part)
	}
}

func TestExtractAnimeSeasonPart_NoSeasonOrPart(t *testing.T) {
	// Simple S1 anime, no season/part in the title
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

func TestComputeEpisodeOffset_NonTVPrequel_ReturnsZero(t *testing.T) {
	// A recap movie or OVA arrives as PREQUEL with Episodes filled in. Adding that would send
	// the search to an episode that does not exist, so it does not enter the count.
	part2 := 2
	for _, format := range []anilist.MediaFormat{anilist.MediaFormatMovie, anilist.MediaFormatOVA, anilist.MediaFormatONA} {
		relations := makeRelationsFmt("PREQUEL", 4, format)
		if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, &part2); got != 0 {
			t.Errorf("format %s: expected offset=0, got %d", format, got)
		}
	}
}

func TestComputeEpisodeOffset_ONAEntryCountsONAPrequelOnly(t *testing.T) {
	// Stone Ocean Part 2 (146722): a streaming series split into ONA parts is one show, so an ONA
	// entry counts its ONA prequel. A movie or OVA prequel still stays out.
	part2 := 2
	if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatONA, makeRelationsFmt("PREQUEL", 12, anilist.MediaFormatONA), &part2); got != 12 {
		t.Errorf("ONA entry, ONA prequel: expected offset=12, got %d", got)
	}
	for _, format := range []anilist.MediaFormat{anilist.MediaFormatMovie, anilist.MediaFormatOVA} {
		if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatONA, makeRelationsFmt("PREQUEL", 4, format), &part2); got != 0 {
			t.Errorf("ONA entry, %s prequel: expected offset=0, got %d", format, got)
		}
	}
}

func TestComputeEpisodeOffset_ONAEntryPrefersTheTVPrequel(t *testing.T) {
	// An ONA "Part 2" with both an ONA side story and its TV Part 1 as PREQUEL: the TV one is the
	// count, whatever order AniList lists the edges in.
	part2, three, twelve := 2, 3, 12
	ona := anilist.MediaRelationEdge{RelationType: "PREQUEL", Node: anilist.MediaRelationNode{Episodes: &three, Format: anilist.MediaFormatONA}}
	tv := anilist.MediaRelationEdge{RelationType: "PREQUEL", Node: anilist.MediaRelationNode{Episodes: &twelve, Format: anilist.MediaFormatTV}}
	for name, edges := range map[string][]anilist.MediaRelationEdge{"ONA first": {ona, tv}, "TV first": {tv, ona}} {
		if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatONA, anilist.MediaRelations{Edges: edges}, &part2); got != 12 {
			t.Errorf("%s: expected the TV prequel's 12, got %d", name, got)
		}
	}
}

func TestComputeEpisodeOffset_SnKS3P2_Returns12(t *testing.T) {
	// Anilist id 104578: PREQUEL = SnK S3 (12 episodes)
	part2 := 2
	relations := makeRelations("PREQUEL", 12)
	offset := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, &part2)
	if offset != 12 {
		t.Errorf("expected offset=12, got %d", offset)
	}
}

func TestComputeEpisodeOffset_MushokuTenseiIIP2_Returns13(t *testing.T) {
	// Anilist id 166873: PREQUEL = MT II (13 episodes)
	part2 := 2
	relations := makeRelations("PREQUEL", 13)
	offset := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, &part2)
	if offset != 13 {
		t.Errorf("expected offset=13, got %d", offset)
	}
}

func TestComputeEpisodeOffset_NoPartGate_ReturnsZero(t *testing.T) {
	// Without part >= 2, offset must be zero (prevents Kaguya S2 from getting a wrong offset)
	relations := makeRelations("PREQUEL", 12)

	// part=nil
	if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, nil); got != 0 {
		t.Errorf("expected 0 for nil part, got %d", got)
	}
	// part=1
	part1 := 1
	if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, &part1); got != 0 {
		t.Errorf("expected 0 for part=1, got %d", got)
	}
}

func TestComputeEpisodeOffset_NoPrequel_ReturnsZero(t *testing.T) {
	// Relations without PREQUEL: no offset
	part2 := 2
	relations := makeRelations("SEQUEL", 12)
	if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, &part2); got != 0 {
		t.Errorf("expected 0 without PREQUEL, got %d", got)
	}
}

func TestComputeEpisodeOffset_KaguySa2_NoPart_ReturnsZero(t *testing.T) {
	// Kaguya S2 (id 112641): part=nil → zero offset (mandatory gate)
	relations := makeRelations("PREQUEL", 12)
	if got := daemon.ComputeEpisodeOffset(anilist.MediaFormatTV, relations, nil); got != 0 {
		t.Errorf("expected 0 for Kaguya S2 (no part), got %d", got)
	}
}

// "Cour N" is a part, never a season (decisions.md #10). Anilist id 141534: english = "Mushoku
// Tensei: Jobless Reincarnation Cour 2 - Eris the Goblin Slayer" (the second half of season 1),
// romaji = "… Part 2 - Eris no Goblin Toubatsu". Reading the cour as season 2 sent the pack path
// a season filter that the release ("… Honki Dasu Part 2 - Eris the Goblin Slayer [1080p]", no
// season marker) can't pass, so the anime found nothing (2026-11-07).
func TestExtractAnimeSeasonPart_MushokuEris_CourIsNotASeason(t *testing.T) {
	title := makeTitle(
		"Mushoku Tensei: Jobless Reincarnation Cour 2 - Eris the Goblin Slayer",
		"Mushoku Tensei: Isekai Ittara Honki Dasu Part 2 - Eris no Goblin Toubatsu",
	)
	season, part := daemon.ExtractAnimeSeasonPart(title, nil)
	if season != nil {
		t.Errorf("expected no season (Cour 2 is a part), got %d", *season)
	}
	if part == nil || *part != 2 {
		t.Errorf("expected part=2, got %v", part)
	}
	if s := nyaa.ExtractSeason("NieR:Automata Ver1.1a Cour 2"); s != nil {
		t.Errorf("ExtractSeason must not read a cour as a season, got %d", *s)
	}
}

// Anilist id 210482: the synonym "JoJo's Bizarre Adventure: Part 7–Steel Ball Run" is the English
// title of the manga it adapts. Its "Part 7" numbers the manga's story arcs, not a cour of the anime,
// and a part 7 rejected every release (none says "Part 7"). A synonym that names a non-anime
// relation is the source's name, not this anime's.
func TestExtractAnimeSeasonPart_SteelBallRun_SourceTitleSynonymIsNotAPart(t *testing.T) {
	media := anilist.Media{
		Title:    makeTitle("STEEL BALL RUN JoJo's Bizarre Adventure 2nd - 3rd STAGE", "JoJo no Kimyou na Bouken: Steel Ball Run - 2nd & 3rd STAGE"),
		Synonyms: []string{"JoJo's Bizarre Adventure: Part 7–Steel Ball Run", "SBR"},
		Relations: anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{{
			RelationType: "ADAPTATION",
			Node: anilist.MediaRelationNode{
				Title:  makeTitle("JoJo's Bizarre Adventure: Part 7–Steel Ball Run", "JoJo no Kimyou na Bouken: Steel Ball Run"),
				Format: anilist.MediaFormatManga,
			},
		}}},
	}
	season, part := daemon.ExtractAnimeSeasonPart(media.Title, media.OwnSynonyms())

	if season != nil {
		t.Errorf("expected no season, got %v", *season)
	}
	if part != nil {
		t.Errorf("expected no part (Part 7 is the manga's arc), got %v", *part)
	}
}

// The same synonym is kept when no relation is a source with that title: an anime's own synonym
// still feeds the part (OwnSynonyms only drops a source's name).
func TestOwnSynonyms_KeepsSynonymWithoutMatchingSource(t *testing.T) {
	media := anilist.Media{
		Synonyms: []string{"Some Show Part 2"},
		Relations: anilist.MediaRelations{Edges: []anilist.MediaRelationEdge{{
			RelationType: "PREQUEL",
			Node:         anilist.MediaRelationNode{Title: makeTitle("Some Show Part 2", ""), Format: anilist.MediaFormatTV},
		}}},
	}
	if got := media.OwnSynonyms(); len(got) != 1 {
		t.Fatalf("an anime relation's title is not a source name, got %v", got)
	}
}
