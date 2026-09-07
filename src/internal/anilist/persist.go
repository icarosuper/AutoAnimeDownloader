package anilist

import (
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"AutoAnimeDownloader/src/internal/logger"
)

// O snapshot em disco existe para UMA coisa: manter as telas e o passe funcionando quando a
// AniList sai do ar, inclusive depois de um restart. Ele NAO e uma frente de leitura — nada
// aqui e servido enquanto a API responde.
//
// Como isso e garantido: as tres colecoes de MediaList voltam do disco com TTL ZERO, ou seja
// visiveis so para getStale, que so roda quando a requisicao falhou. O caminho feliz continua
// indo a rede exatamente como antes, e nenhuma tela passa a mostrar dado mais velho do que
// mostrava. A excecao e o seriesCache, que volta com o TTL cheio de proposito: o que entra nele
// e elo de anime FINISHED com contagem de episodios, dado imutavel por construcao (ver
// recordLink) — nao ha o que vencer.
type snapshot struct {
	SavedAt time.Time `json:"saved_at"`
	// Chaveados exatamente como em memoria (usuario + separador + statuses, ou usuario +
	// separador + media id). O separador e um byte nulo, que o encoder do Go escreve como
	// \u0000 e le de volta igual - a chave sobrevive ao round-trip sem tratamento.
	FrontendList map[string][]MediaList `json:"frontend_list,omitempty"`
	PassList     map[string][]MediaList `json:"pass_list,omitempty"`
	MediaEntries map[string][]MediaList `json:"media_entries,omitempty"`
	// CustomLists entra porque e ela que carrega a exclusao. Sem ela no fallback, um anime que
	// o usuario pos numa lista excluida volta a parecer nao-excluido enquanto a AniList estiver
	// fora — e o passe baixa o que o usuario mandou ignorar.
	CustomLists map[string]map[int]CustomLists `json:"custom_lists,omitempty"`
	// MediaByID cobre os avulsos, que nao aparecem em lista de conta nenhuma: sem ele a lista
	// da tela continua encolhendo — a parte dela que e avulso — com a AniList fora do ar.
	MediaByID map[string]*MediaList `json:"media_by_id,omitempty"`
	Series    map[string]seriesLink `json:"series,omitempty"`
}

// persistDebounce coalesce as escritas. Sem ele cada abertura da tela de detalhe reescreveria o
// snapshot inteiro, e um passe reescreveria uma vez por conta mais uma por caminhada de serie.
const persistDebounce = 5 * time.Second

var (
	persistMu    sync.Mutex
	persistSave  func([]byte) error
	persistTimer *time.Timer
	// snapshotAt e o SavedAt do que esta em disco, em unix. Zero = nunca gravado nesta
	// instalacao. O frontend usa para dizer de quando e o cache que esta vendo.
	snapshotAt atomic.Int64
)

// EnablePersistence liga o snapshot e carrega o que estiver em disco. Chamada uma vez no boot do
// daemon; sem ela o pacote se comporta exatamente como antes (todo cache morre no restart), que
// e o que os testes unitarios querem.
//
// Erro de leitura ou snapshot corrompido nao aborta nada: comeca sem cache e o proximo fetch
// bem-sucedido regrava o arquivo.
func EnablePersistence(load func() ([]byte, error), save func([]byte) error) {
	persistMu.Lock()
	persistSave = save
	persistMu.Unlock()

	data, err := load()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to read the AniList cache snapshot; starting with an empty cache")
		return
	}
	if len(data) == 0 {
		return
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		logger.Logger.Warn().Err(err).Msg("Discarding a corrupt AniList cache snapshot; starting with an empty cache")
		return
	}

	frontendListCache.restore(snap.FrontendList, 0)
	passListCache.restore(snap.PassList, 0)
	mediaEntryCache.restore(snap.MediaEntries, 0)
	customListsCache.restore(snap.CustomLists, 0)
	mediaByIDCache.restore(snap.MediaByID, 0)
	seriesCache.restore(snap.Series, seriesTTL)
	snapshotAt.Store(snap.SavedAt.Unix())

	logger.Logger.Info().
		Time("saved_at", snap.SavedAt).
		Int("frontend_lists", len(snap.FrontendList)).
		Int("pass_lists", len(snap.PassList)).
		Int("media_entries", len(snap.MediaEntries)).
		Int("series_links", len(snap.Series)).
		Msg("Loaded the AniList cache snapshot from disk")
}

// SnapshotSavedAt e a data do snapshot em disco, ou zero se nao ha nenhum. E o que responde
// "servindo do cache de quando?" na tela.
func SnapshotSavedAt() time.Time {
	if unix := snapshotAt.Load(); unix > 0 {
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

// markCacheDirty agenda a gravacao. Barato de chamar em excesso: com um timer pendente ele nao
// faz nada, e sem persistencia ligada tambem nao.
func markCacheDirty() {
	persistMu.Lock()
	defer persistMu.Unlock()
	if persistSave == nil || persistTimer != nil {
		return
	}
	persistTimer = time.AfterFunc(persistDebounce, flushCache)
}

func flushCache() {
	persistMu.Lock()
	persistTimer = nil
	save := persistSave
	persistMu.Unlock()
	if save == nil {
		return
	}

	now := time.Now()
	data, err := json.Marshal(snapshot{
		SavedAt:      now,
		FrontendList: frontendListCache.snapshot(),
		PassList:     passListCache.snapshot(),
		MediaEntries: mediaEntryCache.snapshot(),
		CustomLists:  customListsCache.snapshot(),
		MediaByID:    mediaByIDCache.snapshot(),
		Series:       seriesCache.snapshot(),
	})
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to marshal the AniList cache snapshot")
		return
	}

	// Falha de gravacao nao propaga: o cache em disco e conveniencia, e quem chamou estava
	// servindo uma requisicao que nao tem nada a ver com isso.
	if err := save(data); err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to write the AniList cache snapshot")
		return
	}
	snapshotAt.Store(now.Unix())
}

// disablePersistence desliga e cancela a gravacao pendente. Existe para o isolamento dos
// testes: um snapshot agendado que dispara depois do teste terminar escreveria no diretorio
// temporario de outro teste. Em producao EnablePersistence roda uma vez no boot e ninguem
// desliga.
func disablePersistence() {
	persistMu.Lock()
	defer persistMu.Unlock()
	persistSave = nil
	if persistTimer != nil {
		persistTimer.Stop()
		persistTimer = nil
	}
	snapshotAt.Store(0)
}

// stampAiringAt converte o TimeUntilAiring (relativo ao instante da busca) em AiringAt
// (absoluto) antes de guardar, e e o que torna o cache utilizavel para decidir se um episodio
// ja foi ao ar.
//
// Sem isso o TimeUntilAiring congela no valor do momento da busca, e como TODO gate de "ja
// passou" le esse campo (daemon/episodes.go, anilist/episodes.go, daemon/manual_download.go),
// um passe servido do cache concluiria para sempre que nada novo estreou. Com o instante
// absoluto guardado, rebaseAiring devolve a contagem certa na hora de servir — e o episodio que
// faltava dez minutos quando a AniList caiu vira "ja foi ao ar" sozinho, sem regra nova.
//
// So preenche o que esta zerado: a query por media id ja pede airingAt de verdade, e o da API e
// melhor que o nosso. As do passe e do frontend nao pedem, e e nessas que isto vale.
func stampAiringAt(list []MediaList, now time.Time) {
	unix := now.Unix()
	for i := range list {
		media := &list[i].Media
		for j := range media.AiringSchedule.Nodes {
			node := &media.AiringSchedule.Nodes[j]
			if node.AiringAt == 0 {
				node.AiringAt = unix + int64(node.TimeUntilAiring)
			}
		}
		if media.NextAiringEpisode != nil && media.NextAiringEpisode.AiringAt == 0 {
			media.NextAiringEpisode.AiringAt = unix + int64(media.NextAiringEpisode.TimeUntilAiring)
		}
	}
}

// rebaseAiring e o inverso de stampAiringAt: recalcula o TimeUntilAiring a partir do instante
// absoluto, para que uma entrada guardada ontem nao afirme que faltam as mesmas seis horas de
// ontem. Chamada em TODO caminho que serve do cache.
//
// Trabalha sobre copias dos campos que reescreve, nunca sobre a entrada guardada: reescrever a
// guardada faria o proximo leitor rebasear um valor ja rebaseado.
func rebaseAiring(list []MediaList, now time.Time) {
	unix := now.Unix()
	for i := range list {
		media := &list[i].Media
		nodes := make([]AiringNode, len(media.AiringSchedule.Nodes))
		copy(nodes, media.AiringSchedule.Nodes)
		for j := range nodes {
			if nodes[j].AiringAt != 0 {
				nodes[j].TimeUntilAiring = int(nodes[j].AiringAt - unix)
			}
		}
		media.AiringSchedule.Nodes = nodes

		if media.NextAiringEpisode != nil && media.NextAiringEpisode.AiringAt != 0 {
			next := *media.NextAiringEpisode
			next.TimeUntilAiring = int(next.AiringAt - unix)
			media.NextAiringEpisode = &next
		}
	}
}

// staleList devolve a entrada vencida de um cache de MediaList pronta para uso: copia rasa (o
// chamador sobrescreve CustomLists) e contagem de estreia recalculada.
func staleList(cache *ttlCache[[]MediaList], key string) ([]MediaList, bool) {
	stored, ok := cache.getStale(key)
	if !ok {
		return nil, false
	}
	out := append([]MediaList(nil), stored...)
	rebaseAiring(out, time.Now())
	return out, true
}

// storeList guarda a lista para o fallback: copia rasa, para que a sobrescrita de CustomLists
// do chamador nao alcance o que foi guardado, e AiringAt preenchido.
// Guarda lista vazia tambem: "esta conta nao tem nada nesses status" e resposta legitima, e
// nao guardar faria o poll de 30s ir a rede para sempre em quem tem a lista vazia.
func storeList(cache *ttlCache[[]MediaList], key string, list []MediaList, ttl time.Duration) {
	stored := append([]MediaList(nil), list...)
	stampAiringAt(stored, time.Now())
	cache.set(key, stored, ttl)
	markCacheDirty()
}

// staleMedia devolve o avulso vencido com a contagem de estreia recalculada. Devolve ok=true
// para uma entrada nil guardada: nil ali significa "a AniList nao conhece este id", que e
// resposta, nao ausencia de resposta.
func staleMedia(key string) (*MediaList, bool) {
	stored, ok := mediaByIDCache.getStale(key)
	if !ok {
		return nil, false
	}
	out := copyMediaList(stored)
	if out != nil {
		one := []MediaList{*out}
		rebaseAiring(one, time.Now())
		out = &one[0]
	}
	return out, true
}

// seriesKey e a chave do seriesCache. Existe para que persist.go e series.go concordem sobre o
// formato sem repetir o Itoa em varios lugares.
func seriesKey(mediaID int) string { return strconv.Itoa(mediaID) }
