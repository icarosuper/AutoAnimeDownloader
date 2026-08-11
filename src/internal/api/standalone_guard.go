package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// Motivos pelos quais um anime nao pode ser adicionado como avulso. Sao tambem os codigos de
// erro do 409 do POST e as chaves de tooltip da tela de busca — um campo so, nao quatro
// booleanos, porque sao mutuamente exclusivos por precedencia e o front precisa de um rotulo
// unico por card.
const (
	blockReasonBlacklist  = "blacklist"
	blockReasonStandalone = "standalone"
	blockReasonTracked    = "tracked"
	blockReasonDownloaded = "downloaded"
)

// standaloneGuard responde a unica pergunta que o POST e a busca fazem: este anime pode ser
// adicionado como avulso?
//
// Uma funcao de bloqueio, dois consumidores. E a mesma funcao nos dois porque "o front nao
// deixa clicar" e "o back devolve erro" precisam concordar por construcao — duas definicoes
// produziriam um card cinza que o backend aceita, ou o inverso.
//
// Nenhuma query nova: os quatro sinais ja existem e tres deles ja vem de cache — o arquivo de
// avulsos e o episodes.json sao locais, tracked/blacklisted saem de fetchAniListEntries
// (frontendListCache, 60s) e GetCustomListsMap (5min), exatamente o par que GET /animes monta.
type standaloneGuard struct {
	standalone  map[int]bool // standalone_animes
	downloaded  map[int]int  // mediaID → nº de registros em episodes.json
	tracked     map[int]bool // snapshot do que o daemon PROCESSA (ver newStandaloneGuard)
	blacklisted map[int]bool // customLists ∩ excluded_lists
}

// blockReason devolve "" quando o anime pode ser adicionado, senao um dos quatro motivos.
//
// Precedencia: blacklist > avulso > lista > baixado. Blacklist vem primeiro porque e o unico
// motivo em que adicionar mudaria o comportamento PARA PIOR: um blacklisted em status fora de
// download_statuses escapa do searchAnilist, o registro avulso sobrevive ao merge e o filtro do
// usuario e contornado (o MediaList sintetico de GetMediaByID tem CustomLists nulo, entao
// animeIsInExcludedList nunca dispara nele). Os outros tres sao inocuos — os registros
// existentes ja fazem o loop pular tudo — e a mensagem e so clareza.
func (g standaloneGuard) blockReason(mediaID, totalEpisodes int) string {
	switch {
	case g.blacklisted[mediaID]:
		return blockReasonBlacklist
	case g.standalone[mediaID]:
		return blockReasonStandalone
	case g.tracked[mediaID]:
		return blockReasonTracked
	// So bloqueia com contagem CONHECIDA: um anime de 24 episodios com 12 registros (o limite
	// por anime) nao e "ja baixado", e um total desconhecido nao autoriza afirmar nada.
	case totalEpisodes > 0 && g.downloaded[mediaID] >= totalEpisodes:
		return blockReasonDownloaded
	}
	return ""
}

// newStandaloneGuard monta o guard a partir do estado que o servidor ja tem em maos.
//
// tracked sai de fetchAniListEntries, e nao do searchAnilist do daemon: sao consultas
// diferentes com OS MESMOS DOIS FILTROS (download_statuses server-side + MediaStatusAllowed), e
// a do pacote api e a que ja tem cache no caminho do frontend. A definicao e "aparece no
// conjunto que o daemon PROCESSA", nao "existe entrada na AniList" — um anime em PLANNING com
// download_statuses = [CURRENT] esta numa lista e o daemon o ignora, e adiciona-lo como avulso
// e o caso de uso mais obvio da feature.
func newStandaloneGuard(fm FileManagerInterface, config *files.Config) (standaloneGuard, error) {
	guard := standaloneGuard{
		standalone:  map[int]bool{},
		downloaded:  map[int]int{},
		tracked:     map[int]bool{},
		blacklisted: map[int]bool{},
	}

	standaloneIDs, err := fm.LoadStandaloneAnimes()
	if err != nil {
		return guard, err
	}
	for _, id := range standaloneIDs {
		guard.standalone[id] = true
	}

	episodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return guard, err
	}
	for _, ep := range episodes {
		if ep.AnimeID != 0 {
			guard.downloaded[ep.AnimeID]++
		}
	}

	excluded := make(map[string]bool, len(config.ExcludedLists))
	for _, name := range config.ExcludedLists {
		excluded[name] = true
	}

	for _, username := range config.AnilistUsernames {
		// nil significa busca falhada (ver fetchAniListEntries). Tratar isso como "nada
		// acompanhado" e o comportamento certo aqui: o front e best-effort e o POST recusa de
		// novo com o snapshot da proxima chamada.
		for _, ml := range fetchAniListEntries(username, config.DownloadStatuses, config.DownloadMediaStatuses) {
			guard.tracked[ml.Media.Id] = true
			if isInExcludedList(ml.CustomLists, excluded) {
				guard.blacklisted[ml.Media.Id] = true
			}
		}
	}

	return guard, nil
}

func isInExcludedList(customLists anilist.CustomLists, excluded map[string]bool) bool {
	for name, inList := range customLists {
		if inList && excluded[name] {
			return true
		}
	}
	return false
}
