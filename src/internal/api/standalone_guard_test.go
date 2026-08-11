package api

import "testing"

// TestBlockReason cobre a tabela inteira. O mesmo blockReason serve o 409 do POST e o
// block_reason do resultado da busca, de proposito: "o front nao deixa clicar" e "o back
// devolve erro" precisam concordar por construcao, nao por disciplina.
func TestBlockReason(t *testing.T) {
	cases := []struct {
		name          string
		guard         standaloneGuard
		mediaID       int
		totalEpisodes int
		want          string
	}{
		{
			name:  "nada bloqueia",
			guard: standaloneGuard{},
			want:  "",
		},
		{
			name:    "ja avulso",
			guard:   standaloneGuard{standalone: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonStandalone,
		},
		{
			name:    "na lista processada",
			guard:   standaloneGuard{tracked: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonTracked,
		},
		{
			name:    "em lista excluida",
			guard:   standaloneGuard{blacklisted: map[int]bool{21: true}},
			mediaID: 21,
			want:    blockReasonBlacklist,
		},
		{
			name:          "24 registros de 24 episodios",
			guard:         standaloneGuard{downloaded: map[int]int{21: 24}},
			mediaID:       21,
			totalEpisodes: 24,
			want:          blockReasonDownloaded,
		},
		{
			// 12 e o limite por anime, nao "ja baixado" — e o caso que a regra batch<->limite
			// passa a completar.
			name:          "12 registros de 24 episodios nao e ja baixado",
			guard:         standaloneGuard{downloaded: map[int]int{21: 12}},
			mediaID:       21,
			totalEpisodes: 24,
			want:          "",
		},
		{
			name:          "total desconhecido nunca bloqueia",
			guard:         standaloneGuard{downloaded: map[int]int{21: 300}},
			mediaID:       21,
			totalEpisodes: 0,
			want:          "",
		},
		{
			// A precedencia importa: blacklist e o unico motivo em que adicionar mudaria o
			// comportamento para pior (o registro avulso contorna o filtro do usuario).
			name: "avulso E em lista excluida devolve blacklist",
			guard: standaloneGuard{
				standalone:  map[int]bool{21: true},
				blacklisted: map[int]bool{21: true},
			},
			mediaID: 21,
			want:    blockReasonBlacklist,
		},
		{
			name: "avulso E na lista devolve standalone",
			guard: standaloneGuard{
				standalone: map[int]bool{21: true},
				tracked:    map[int]bool{21: true},
			},
			mediaID: 21,
			want:    blockReasonStandalone,
		},
		{
			name: "na lista E ja baixado devolve tracked",
			guard: standaloneGuard{
				tracked:    map[int]bool{21: true},
				downloaded: map[int]int{21: 24},
			},
			mediaID:       21,
			totalEpisodes: 24,
			want:          blockReasonTracked,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.guard.blockReason(tc.mediaID, tc.totalEpisodes); got != tc.want {
				t.Fatalf("blockReason(%d, %d) = %q, quero %q", tc.mediaID, tc.totalEpisodes, got, tc.want)
			}
		})
	}
}

// TestBlockReason_PlanningIsNotTracked: um anime em PLANNING com download_statuses=[CURRENT]
// esta numa lista da AniList e o daemon o ignora — adiciona-lo como avulso e o caso de uso mais
// obvio da feature, nao um erro a bloquear. Se este teste virar "tracked", a definicao de
// "acompanhado" regrediu para "existe entrada na AniList".
func TestBlockReason_PlanningIsNotTracked(t *testing.T) {
	// tracked sai do snapshot que o daemon PROCESSA (fetchAniListEntries, ja filtrado por
	// download_statuses), entao a entrada em PLANNING simplesmente nao esta la.
	guard := standaloneGuard{tracked: map[int]bool{}}

	if got := guard.blockReason(21, 12); got != "" {
		t.Fatalf("anime em PLANNING fora de download_statuses deve poder ser adicionado, veio %q", got)
	}
}
