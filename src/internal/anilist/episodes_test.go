package anilist

import "testing"

func mediaList(status MediaStatus, totalEpisodes *int, next *int, scheduled ...AiringNode) MediaList {
	ml := MediaList{Media: Media{Id: 21, Status: status, Episodes: totalEpisodes}}
	ml.Media.AiringSchedule.Nodes = scheduled
	if next != nil {
		ml.Media.NextAiringEpisode = &AiringNode{Episode: *next, TimeUntilAiring: 3600}
	}
	return ml
}

func numbers(nodes []AiringNode) []int {
	out := make([]int, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Episode)
	}
	return out
}

// A agenda do One Piece comeca no 1123 (a AniList descarta as entradas antigas), e nextAiringEpisode
// e a unica pista do que ja foi ao ar. Sem sintetizar, os 1122 primeiros nao existiriam.
func TestEpisodeList_FillsGapBelowTheAniListWindow(t *testing.T) {
	next := 1174
	ml := mediaList(MediaStatusReleasing, nil, &next,
		AiringNode{ID: 416348, Episode: 1123, TimeUntilAiring: -100, AiringAt: 42},
		AiringNode{ID: 416349, Episode: 1124, TimeUntilAiring: -50},
		AiringNode{ID: 416400, Episode: 1174, TimeUntilAiring: 3600},
	)

	got := EpisodeList(ml, 1)
	if len(got) != 1174 {
		t.Fatalf("quero 1174 episodios (1..1173 no ar + 1174 futuro), veio %d", len(got))
	}
	if got[0].Episode != 1 || got[0].TimeUntilAiring >= 0 {
		t.Errorf("episodio 1 deve ser sintetizado como ja exibido, veio %+v", got[0])
	}
	if got[1122].Episode != 1123 || got[1122].AiringAt != 42 {
		t.Errorf("onde a agenda existe o no REAL deve ser usado (traz airingAt), veio %+v", got[1122])
	}
	if last := got[1173]; last.Episode != 1174 || last.TimeUntilAiring <= 0 {
		t.Errorf("o episodio ainda por vir deve entrar como futuro, veio %+v", last)
	}
}

// Anime antigo/finalizado volta com airingSchedule VAZIO: a lista tem de sair de media.episodes,
// senao nada dele e baixavel.
func TestEpisodeList_FinishedAnimeWithEmptySchedule(t *testing.T) {
	total := 25
	got := EpisodeList(mediaList(MediaStatusFinished, &total, nil), 1)
	if len(got) != 25 {
		t.Fatalf("quero 25 episodios, veio %d: %v", len(got), numbers(got))
	}
	for i, n := range got {
		if n.Episode != i+1 || n.TimeUntilAiring > 0 {
			t.Fatalf("episodio %d inesperado: %+v", i+1, n)
		}
	}
}

// fromEpisode e progresso+1: o que o usuario ja assistiu nao entra na lista.
func TestEpisodeList_StartsAtFromEpisode(t *testing.T) {
	total := 12
	got := numbers(EpisodeList(mediaList(MediaStatusFinished, &total, nil), 10))
	want := []int{10, 11, 12}
	if len(got) != len(want) {
		t.Fatalf("quero %v, veio %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("quero %v, veio %v", want, got)
		}
	}
}

// Num RELEASING media.episodes e a contagem PREVISTA da temporada: usa-la como "ultimo no ar"
// mandaria o daemon buscar episodio que ainda nao existe. O nextAiringEpisode entra na lista, mas
// como FUTURO (o daemon o pula, a tela o mostra).
func TestEpisodeList_ReleasingIgnoresPredictedTotal(t *testing.T) {
	total := 24
	next := 6
	got := EpisodeList(mediaList(MediaStatusReleasing, &total, &next), 1)
	if len(got) != 6 {
		t.Fatalf("quero 5 no ar + o proximo, veio %v", numbers(got))
	}
	for _, n := range got[:5] {
		if n.TimeUntilAiring > 0 {
			t.Fatalf("episodio %d deveria estar no ar: %+v", n.Episode, n)
		}
	}
	if got[5].Episode != 6 || got[5].TimeUntilAiring <= 0 {
		t.Fatalf("o proximo episodio deve entrar como futuro, veio %+v", got[5])
	}
}

// NOT_YET_RELEASED nao tem episodio no ar: a lista traz so o que a agenda promete.
func TestEpisodeList_NotYetReleased(t *testing.T) {
	got := EpisodeList(mediaList(MediaStatusNotYetReleased, nil, nil,
		AiringNode{Episode: 1, TimeUntilAiring: 7200},
	), 1)
	if len(got) != 1 || got[0].Episode != 1 || got[0].TimeUntilAiring <= 0 {
		t.Fatalf("quero apenas o episodio 1 futuro, veio %+v", got)
	}
}
