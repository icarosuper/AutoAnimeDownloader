package anilist

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// memoryStore e o par load/save que EnablePersistence espera, sem tocar em disco. O que se quer
// testar e a semantica do snapshot, nao o writeAtomic do files.
func memoryStore(initial []byte) (load func() ([]byte, error), save func([]byte) error, current *[]byte) {
	data := initial
	return func() ([]byte, error) { return data, nil },
		func(b []byte) error { data = b; return nil },
		&data
}

// frontendListBody monta a resposta de GetFrontendAnimeList com um episodio faltando
// timeUntilAiring segundos para estrear.
func frontendListBody(mediaID, episode, timeUntilAiring int) string {
	return `{"data":{"Page":{"mediaList":[{"id":1,"progress":0,"media":{"id":` +
		strconv.Itoa(mediaID) + `,"title":{"romaji":"Teste"},"status":"RELEASING","airingSchedule":{"nodes":[{"episode":` +
		strconv.Itoa(episode) + `,"timeUntilAiring":` + strconv.Itoa(timeUntilAiring) + `}]}}}]}}}`
}

// expire vence a entrada sem apaga-la: e o que faz o proximo get errar o cache e ir a rede,
// deixando o valor guardado la para o getStale. Mesmo truque do budget_test.
func expire(key string) {
	stored, _ := frontendListCache.getStale(key)
	frontendListCache.set(key, stored, -time.Second)
}

// TestOutageServesFrontendListFromCache e o coracao da feature: com a AniList fora do ar a lista
// continua saindo, e o chamador FICA SABENDO que ela veio do cache. Sem o segundo fato o
// endpoint /animes dispararia o refresh de orfaos contra uma API que nao responde.
func TestOutageServesFrontendListFromCache(t *testing.T) {
	fail := false
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		if fail {
			return respond(503, `{}`, nil), nil
		}
		return respond(200, frontendListBody(7, 3, 600), nil), nil
	})
	t.Cleanup(restore)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err != nil {
		t.Fatalf("a primeira busca tinha de passar: %v", err)
	}

	// Vence o TTL para que o proximo get erre o cache e va a rede — que agora falha.
	expire("user\x00CURRENT")
	fail = true

	resp, err := GetFrontendAnimeList("user", []string{"CURRENT"})
	if !errors.Is(err, ErrFromCache) {
		t.Fatalf("esperava ErrFromCache, veio %v", err)
	}
	if resp == nil || len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 7 {
		t.Fatalf("o cache tinha de devolver o anime 7, veio %+v", resp)
	}
}

// TestOutageWithoutCacheStillFails: sem entrada guardada nao ha o que servir, e o erro tem de
// subir inteiro. Um fallback que inventa lista vazia faria a tela dizer "voce nao acompanha
// nada" com a AniList fora do ar.
func TestOutageWithoutCacheStillFails(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(503, `{}`, nil), nil
	})
	t.Cleanup(restore)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err == nil || errors.Is(err, ErrFromCache) {
		t.Fatalf("esperava a falha crua da AniList, veio %v", err)
	}
}

// TestCachedAiringCountdownIsRebased e a razao de stampAiringAt existir. O TimeUntilAiring da
// AniList e relativo ao instante da busca: servido cru do cache ele congela, e o passe conclui
// para sempre que nada novo estreou.
//
// Aqui o snapshot diz "faltam 10 minutos" e foi gravado uma hora atras. Depois de servido, a
// contagem tem de estar NEGATIVA — o episodio ja foi ao ar, e e exatamente esse o caso que o
// cache precisa acertar para o passe continuar baixando com a AniList fora.
func TestCachedAiringCountdownIsRebased(t *testing.T) {
	restore := MockAniListDo(nil)
	t.Cleanup(restore)

	savedAt := time.Now().Add(-time.Hour)
	stored := []MediaList{{Media: Media{
		Id:             7,
		AiringSchedule: AiringSchedule{Nodes: []AiringNode{{Episode: 3, TimeUntilAiring: 600}}},
	}}}
	// stampAiringAt no instante da gravacao e o que persist.go faz por storeList.
	stampAiringAt(stored, savedAt)
	frontendListCache.set("k", stored, 0)

	served, ok := staleList(frontendListCache, "k")
	if !ok {
		t.Fatal("a entrada guardada tinha de ser servida")
	}

	got := served[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring
	if got > -3000 || got < -4200 {
		t.Fatalf("esperava a contagem rebaseada para cerca de -3000s (10min faltando, 1h atras), veio %d", got)
	}

	// A entrada GUARDADA nao pode ter sido reescrita: rebasear duas vezes andaria o relogio
	// duas vezes e o proximo leitor veria uma contagem ainda mais negativa.
	if kept, _ := frontendListCache.getStale("k"); kept[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring != 600 {
		t.Fatalf("a entrada guardada foi reescrita pelo rebase: %d", kept[0].Media.AiringSchedule.Nodes[0].TimeUntilAiring)
	}
}

// TestSnapshotSurvivesRestart: o snapshot volta do disco como FALLBACK, nunca como dado fresco.
// E a invariante que separa "sobrevive a queda" de "passou a mostrar dado velho": um restore que
// devolvesse as entradas visiveis ao get faria toda tela servir o cache de ontem sem nem tentar
// a rede.
func TestSnapshotSurvivesRestart(t *testing.T) {
	restore := MockAniListDo(func(*http.Request) (*http.Response, error) {
		return respond(200, frontendListBody(7, 3, 600), nil), nil
	})
	t.Cleanup(restore)

	load, save, current := memoryStore(nil)
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	if _, err := GetFrontendAnimeList("user", []string{"CURRENT"}); err != nil {
		t.Fatalf("a busca tinha de passar: %v", err)
	}
	// A gravacao e agendada; o teste nao espera cinco segundos por ela.
	flushCache()
	if len(*current) == 0 {
		t.Fatal("o snapshot nao foi gravado")
	}

	// Simula o restart: zera todo estado de pacote e recarrega do "disco".
	clearCaches()
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	key := "user\x00CURRENT"
	if _, fresh := frontendListCache.get(key); fresh {
		t.Fatal("entrada restaurada nao pode aparecer como fresca: o caminho feliz deixaria de ir a rede")
	}
	if _, ok := frontendListCache.getStale(key); !ok {
		t.Fatal("entrada restaurada tinha de estar disponivel como fallback")
	}
	if SnapshotSavedAt().IsZero() {
		t.Fatal("a data do snapshot tinha de voltar, e o que a tela mostra no banner")
	}
}

// TestCorruptSnapshotIsDiscarded: arquivo ilegivel comeca sem cache em vez de derrubar o boot. O
// cache existe para o app funcionar pior em vez de nao funcionar; um snapshot ruim que impedisse
// a inicializacao inverteria isso.
func TestCorruptSnapshotIsDiscarded(t *testing.T) {
	restore := MockAniListDo(nil)
	t.Cleanup(restore)

	load, save, _ := memoryStore([]byte("{nao e json"))
	EnablePersistence(load, save)
	t.Cleanup(disablePersistence)

	if !SnapshotSavedAt().IsZero() {
		t.Fatal("snapshot corrompido nao pode registrar data")
	}
	if _, ok := frontendListCache.getStale("user\x00CURRENT"); ok {
		t.Fatal("snapshot corrompido nao pode popular cache")
	}
}
