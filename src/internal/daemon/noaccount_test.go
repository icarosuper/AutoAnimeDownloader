package daemon

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
)

// Conta da AniList deixou de ser obrigatoria: com animes avulsos o app funciona inteiro sem
// nenhuma lista configurada. O unico requisito que sobra e a biblioteca — sem ela nao ha para
// onde baixar.
func TestIsConfigComplete_WithoutAniListAccount(t *testing.T) {
	config := &files.Config{
		AnilistUsernames:   []string{},
		CompletedAnimePath: "/tmp/completed",
	}

	if !isConfigComplete(config) {
		t.Fatal("config sem conta da AniList mas com biblioteca deve ser completa")
	}
}

func TestIsConfigComplete_WithoutLibrary(t *testing.T) {
	config := &files.Config{AnilistUsernames: []string{"user1"}}

	if isConfigComplete(config) {
		t.Fatal("config sem biblioteca nao pode ser completa: nao ha para onde baixar")
	}
}

// TestSearchAnilist_NoAccountsStillProcessesStandalone: sem conta nenhuma o passe ainda tem
// trabalho a fazer — os avulsos. Devolver erro aqui abortaria o passe inteiro e a feature
// nunca rodaria numa instalacao sem AniList.
func TestSearchAnilist_NoAccountsStillProcessesStandalone(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()

	configs := standaloneTestConfig()
	configs.AnilistUsernames = []string{}

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{500}}
	resp, err := searchAnilist(fm, configs, []int{500})
	if err != nil {
		t.Fatalf("sem conta nao e erro: %v", err)
	}
	if len(resp.Data.Page.MediaList) != 1 || resp.Data.Page.MediaList[0].Media.Id != 500 {
		t.Fatalf("o avulso precisa ser processado, veio %+v", resp.Data.Page.MediaList)
	}
}

// TestSearchAnilist_NoLibraryStillFails: a biblioteca continua obrigatoria.
func TestSearchAnilist_NoLibraryStillFails(t *testing.T) {
	configs := standaloneTestConfig()
	configs.CompletedAnimePath = ""

	if _, err := searchAnilist(&mockFileManagerForEpisodes{}, configs, nil); err == nil {
		t.Fatal("quero erro sem biblioteca configurada")
	}
}

// TestManualDownloadEpisode_StandaloneAnime: a tela de detalhe de um avulso mostra os botões de
// download/redownload/replace por episódio. Sem o fallback de GetMediaByID em
// resolveAnimeDetails todos eles falham antes de chegar ao Nyaa — a tela abre e nenhum botão
// funciona.
func TestManualDownloadEpisode_StandaloneAnime(t *testing.T) {
	defer mockAniListRouter(t, `{"data": {"Page": {"mediaList": []}}}`, mediaForAnime500)()
	defer mockEmptyNyaa()()

	configs := standaloneTestConfig()
	configs.AnilistUsernames = []string{}

	// Sem torrent no Nyaa o download falha, mas a mensagem tem de ser sobre o torrent — não
	// sobre o anime não estar em lista nenhuma.
	_, err := ManualDownloadEpisode(torrents.NewFakeBackend(), 500, 1, configs, "")
	if err == nil {
		t.Fatal("sem torrent no Nyaa o download precisa falhar")
	}
	if !strings.Contains(err.Error(), "no torrents found") {
		t.Fatalf("o anime avulso precisa ser resolvido; erro veio de outro lugar: %v", err)
	}
}
