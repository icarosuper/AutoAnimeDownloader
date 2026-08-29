package anilist

import (
	"os"
	"testing"
)

// Smoke contra a AniList de verdade — a janela de agenda que ela devolve é o motivo de
// EpisodeList existir (ver decisions.md #52), e nenhum mock prova que ela continua daquele
// tamanho. Só roda com AAD_LIVE_ANILIST=1, para o `go test ./...` não depender de rede.
func TestLive_EpisodeList(t *testing.T) {
	if os.Getenv("AAD_LIVE_ANILIST") != "1" {
		t.Skip("defina AAD_LIVE_ANILIST=1 para rodar")
	}
	// 21 = ONE PIECE (agenda clipada), 16498 = Shingeki no Kyojin e 20 = NARUTO (agenda vazia).
	for _, id := range []int{21, 16498, 20} {
		ml, err := GetMediaByID(id, PriorityCritical)
		if err != nil || ml == nil {
			t.Fatalf("media %d: %v", id, err)
		}
		eps := EpisodeList(*ml, 1)
		next := 0
		if ml.Media.NextAiringEpisode != nil {
			next = ml.Media.NextAiringEpisode.Episode
		}
		first, last := 0, 0
		if len(eps) > 0 {
			first, last = eps[0].Episode, eps[len(eps)-1].Episode
		}
		t.Logf("media %d: agenda=%d nós, nextAiring=%d → EpisodeList=%d episódios (%d..%d)",
			id, len(ml.Media.AiringSchedule.Nodes), next, len(eps), first, last)
		if len(eps) == 0 {
			t.Errorf("media %d ficou sem episódios", id)
		}
		if first != 1 {
			t.Errorf("media %d deveria começar no episódio 1, começou no %d", id, first)
		}
	}
}
