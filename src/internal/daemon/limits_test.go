package daemon

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/notifications"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

// animeWithEpisodes builds an anime with n aired episode nodes. total is what AniList reports as
// Media.Episodes (nil when totalKnown is false).
func animeWithEpisodes(n int, status anilist.MediaStatus, totalKnown bool, format anilist.MediaFormat) anilist.MediaList {
	title := "Limits Test Anime"
	nodes := make([]anilist.AiringNode, n)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 2000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	media := anilist.Media{
		Id:             777,
		Status:         status,
		Format:         format,
		Title:          anilist.Title{English: &title},
		AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
	}
	if totalKnown {
		total := n
		media.Episodes = &total
	}
	return anilist.MediaList{Id: 777, Status: anilist.MediaListStatusCurrent, Media: media}
}

// searcherFor builds a searcher whose strategies return the given results. batch e multiple viram
// UMA lista (a busca por anime devolve as duas juntas): pack ganha IsBatch, episodio ja vem com
// Episode preenchido por multipleFor.
func searcherFor(batch, multiple, single, movie []nyaa.TorrentResult) nyaaSearcher {
	anime := make([]nyaa.TorrentResult, 0, len(batch)+len(multiple))
	for _, tr := range batch {
		tr.IsBatch = true
		anime = append(anime, tr)
	}
	anime = append(anime, multiple...)

	return nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int, string) []nyaa.TorrentResult { return anime },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, string, int) []nyaa.TorrentResult {
			return single
		},
		searchMovie: func(anilist.Title, bool, string) []nyaa.TorrentResult { return movie },
	}
}

// fakeMagnet builds a magnet with a valid (unique) 40-hex infohash, which FakeBackend requires.
func fakeMagnet(n int) string {
	return "magnet:?xt=urn:btih:" + strings.Repeat("0", 36) + fmt.Sprintf("%04x", n)
}

// multipleFor builds one per-episode result for each episode number in 1..n, as the singles
// side of ScrapNyaaForAnime would.
func multipleFor(n int, sizeBytes int64) []nyaa.TorrentResult {
	out := make([]nyaa.TorrentResult, 0, n)
	for i := 1; i <= n; i++ {
		ep := i
		out = append(out, nyaa.TorrentResult{
			Name:       "ep",
			MagnetLink: fakeMagnet(i),
			Episode:    &ep,
			Size:       sizeBytes,
		})
	}
	return out
}

func limitsConfig() *files.Config {
	return &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   3,
	}
}

// TestWillBatch_FinishedWithinCeilingIgnoresPerAnimeLimit: 26 episódios finalizados casam com o
// batch, e o limite de 12 por anime NÃO se aplica — um batch é um torrent só, limitar registros
// não limitaria nem bytes nem arquivos na biblioteca.
func TestWillBatch_FinishedWithinCeilingIgnoresPerAnimeLimit(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "batch", MagnetLink: fakeMagnet(9001)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 26 {
		t.Errorf("esperava 26 episódios registrados pelo batch, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episódio %d deveria estar marcado como batch", ep.EpisodeNumber)
		}
	}
}

// RELEASING de 1100 episódios BUSCA pack (antes desta spec não buscava, por status), e o pack
// parcial cobre a janela do limite por anime.
func TestEligibility_ReleasingLongSeriesUsesPartialPack(t *testing.T) {
	anime := animeWithEpisodes(1100, anilist.MediaStatusReleasing, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 001-100 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 100 {
		t.Fatalf("esperava os 100 episódios do pack registrados, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episódio %d deveria estar marcado como batch", ep.EpisodeNumber)
		}
	}
}

// Media.Episodes == nil passa a ser elegível: sem comparação de contagem, contagem desconhecida
// deixa de importar.
func TestEligibility_UnknownTotalIsEligible(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 26 {
		t.Errorf("contagem desconhecida deve poder usar pack, obteve %d", len(result.newEpisodes))
	}
}

// Um episódio pendente não busca pack: um pack para um episódio é o caminho de episódio solto.
func TestEligibility_SinglePendingEpisodeDoesNotUsePack(t *testing.T) {
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, multipleFor(1, 0), nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 1 || result.newEpisodes[0].IsBatch {
		t.Errorf("esperava 1 episódio solto, obteve %+v", result.newEpisodes)
	}
}

// O caminho que o tampão cobria, agora sem tampão: o filtro de tamanho esvazia os packs, pickBatches
// devolve vazio, o fluxo cai em episódio solto e o limite por anime VALE.
func TestEligibility_SizeCeilingEmptiesPacksAndLimitApplies(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	configs := limitsConfig()
	configs.MaxBatchTorrentSizeGB = 10
	const gib = int64(1024 * 1024 * 1024)
	searcher := searcherFor(
		[]nyaa.TorrentResult{{Name: "[X] Anime 01-26 remux [1080p]", MagnetLink: fakeMagnet(9002), Size: 300 * gib}},
		multipleFor(26, gib),
		nil, nil,
	)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 12 {
		t.Errorf("esperava 12 episódios individuais, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatal("nenhum episódio deveria vir marcado como batch")
		}
	}
}

// Formato One Piece: RELEASING, Media.Episodes == nil, 1100 episódios no ar. O tamanho da série
// tem de chegar na busca de episódio (é ele que liga o zero-padding da query) — e Media.Episodes
// sozinho não serviria, porque é justamente nil na série longa em andamento.
func TestSingleEpisodeSearch_ReceivesSeriesLength(t *testing.T) {
	anime := animeWithEpisodes(1100, anilist.MediaStatusReleasing, false, "")
	got := 0
	searcher := searcherFor(nil, nil, nil, nil)
	searcher.searchSingleEpisode = func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ string, totalEpisodes int) []nyaa.TorrentResult {
		got = totalEpisodes
		return nil
	}

	processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if got != 1100 {
		t.Errorf("esperava 1100 como tamanho da série na busca de episódio, obteve %d", got)
	}
}

// TestWillBatch_MovieUnaffected: filme continua usando a estratégia de filme.
func TestWillBatch_MovieUnaffected(t *testing.T) {
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, anilist.MediaFormatMovie)
	searcher := searcherFor(nil, nil, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(9004)}})

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 1 || !result.newEpisodes[0].IsBatch {
		t.Errorf("filme deve baixar como torrent único, obteve %+v", result.newEpisodes)
	}
}

// TestEpisodeSizeCeiling_DoesNotAffectBatchChoice: um pack de 40 GiB continua sendo baixado com
// max_episode_torrent_size_gb = 1.5 — os dois tetos são independentes.
func TestEpisodeSizeCeiling_DoesNotAffectBatchChoice(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	configs := limitsConfig()
	configs.MaxEpisodeTorrentSizeGB = 1.5
	const gib = int64(1024 * 1024 * 1024)
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "pack", MagnetLink: fakeMagnet(9003), Size: 40 * gib}}, nil, nil, nil)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 26 {
		t.Errorf("o teto de episódio não deve filtrar o batch, obteve %d episódios", len(result.newEpisodes))
	}
}

func TestFilterBySize(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	results := []nyaa.TorrentResult{
		{Name: "small", Size: 1 * gib},
		{Name: "huge", Size: 300 * gib},
		{Name: "unknown", Size: 0},
		{Name: "medium", Size: 5 * gib},
	}

	if got := filterBySize(results, 0); len(got) != 4 {
		t.Errorf("teto 0 não deve filtrar nada, sobraram %d", len(got))
	}

	got := filterBySize(results, 10)
	if len(got) != 3 {
		t.Fatalf("esperava 3 resultados, obteve %d (%+v)", len(got), got)
	}
	// Size == 0 fica (parsing quebrado não pode virar paralisação) e a ordem é preservada.
	want := []string{"small", "unknown", "medium"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, got[i].Name)
		}
	}
}

func TestFilterBySeeders(t *testing.T) {
	results := []nyaa.TorrentResult{
		{Name: "alive", Seeders: "412"},
		{Name: "dead", Seeders: "0"},
		{Name: "unknown", Seeders: "-"},
		{Name: "weak", Seeders: "3"},
	}

	if got := filterBySeeders(results, 0); len(got) != 4 {
		t.Errorf("piso 0 não deve filtrar nada, sobraram %d", len(got))
	}

	// Default: só o literalmente morto sai (e "-" conta como 0).
	got := filterBySeeders(results, 1)
	want := []string{"alive", "weak"}
	if len(got) != len(want) {
		t.Fatalf("esperava %d resultados, obteve %d (%+v)", len(want), len(got), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, got[i].Name)
		}
	}

	if got := filterBySeeders(results, 5); len(got) != 1 || got[0].Name != "alive" {
		t.Errorf("piso 5 deveria sobrar só o alive, obteve %+v", got)
	}
}

// --- Guarda de espaço em disco ---

func TestCheckDiskSpace(t *testing.T) {
	dir := t.TempDir()

	if err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskPercent: 1}); err != nil {
		t.Errorf("com 1%% exigido esperava nil, obteve %v", err)
	}
	// 100% livre é impossível num volume em uso: força o caminho "abaixo do teto".
	err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskPercent: 100})
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("esperava ErrInsufficientDiskSpace, obteve %v", err)
	}
	if err := checkDiskSpace(&files.Config{CompletedAnimePath: dir, MinFreeDiskPercent: 0}); err != nil {
		t.Errorf("0 desliga a guarda, obteve %v", err)
	}
	// Erro de statfs não bloqueia.
	missing := filepath.Join(dir, "nao-existe")
	if err := checkDiskSpace(&files.Config{CompletedAnimePath: missing, MinFreeDiskPercent: 100}); err != nil {
		t.Errorf("falha de statfs não deve bloquear, obteve %v", err)
	}
}

// diskFullConfig devolve uma config cuja guarda de disco sempre barra.
func diskFullConfig(t *testing.T) *files.Config {
	t.Helper()
	configs := limitsConfig()
	configs.CompletedAnimePath = t.TempDir()
	configs.MinFreeDiskPercent = 100
	return configs
}

func TestAttemptDownloadWithRetries_DiskFullDoesNotCallAdd(t *testing.T) {
	backend := torrents.NewFakeBackend()
	hash := attemptDownloadWithRetries(diskFullConfig(t), backend, []string{fakeMagnet(1), fakeMagnet(2)}, "ep")

	if hash != "" {
		t.Errorf("esperava hash vazio com disco cheio, obteve %q", hash)
	}
	if len(backend.List()) != 0 {
		t.Errorf("backend.Add não deve ser chamado nenhuma vez, cliente tem %d torrent(s)", len(backend.List()))
	}
}

func TestProcessAnimeEpisodes_DiskFullNotifiesNoDiskSpaceReason(t *testing.T) {
	bodies := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
	}))
	defer srv.Close()

	configs := diskFullConfig(t)
	configs.Notifications = files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
		Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
		Body: "{{reason}}", Events: []string{"download_failed"},
	}}}

	anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, false, "")
	searcher := searcherFor(nil, nil, []nyaa.TorrentResult{{MagnetLink: fakeMagnet(1)}}, nil)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)
	if len(result.newEpisodes) != 0 {
		t.Errorf("nada deve ser registrado com disco cheio, obteve %d", len(result.newEpisodes))
	}

	select {
	case body := <-bodies:
		if !strings.Contains(body, notifications.ReasonNoDiskSpace) {
			t.Errorf("esperava a razão %q no webhook, obteve %q", notifications.ReasonNoDiskSpace, body)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("nenhum webhook de falha recebido")
	}
}

// TestHandleSavedEpisodes_DiskFullStillPrunes: disco cheio barra o Add, não o passe — a poda é
// justamente o que libera espaço.
func TestHandleSavedEpisodes_DiskFullStillPrunes(t *testing.T) {
	configs := diskFullConfig(t)
	configs.DeleteWatchedEpisodes = true

	saved := []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: "h1"}, {EpisodeNumber: 2, EpisodeHash: "h2"}}
	fm := &mockFileManagerForEpisodes{}
	backend := fakeWithTorrents("h1", "h2")

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes: saved,
		keysToDelete:  []files.EpisodeKey{{Episode: 1}, {Episode: 2}},
	})

	if !containsID(fm.deletedEpisodeKeys, files.EpisodeKey{Episode: 1}) || !containsID(fm.deletedEpisodeKeys, files.EpisodeKey{Episode: 2}) {
		t.Errorf("a poda deve rodar com disco cheio, apagados = %v", fm.deletedEpisodeKeys)
	}
}

// TestOrganizeTorrent_PartiallyOrganizedSkipsWebhook: registros novos criados para um torrent já
// organizado (o caminho da migração da regra de batch) não redisparam DownloadCompleted.
func TestOrganizeTorrent_PartiallyOrganizedSkipsWebhook(t *testing.T) {
	events := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- string(body)
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "ep01.mkv"), []byte("v"), 0644); err != nil {
		t.Fatal(err)
	}
	const hash = "0123456789abcdef0123456789abcdef01234567"
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hash, dataDir)

	fm := &orchestrationFM{
		saved: []files.EpisodeStruct{
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 1, IsBatch: true, LibraryPaths: []string{"/library/My Anime/ep01.mkv"}},
			{EpisodeHash: hash, AnimeName: "My Anime", EpisodeNumber: 2, IsBatch: true},
		},
		configs: &files.Config{
			CompletedAnimePath: t.TempDir(),
			Notifications: files.NotificationsConfig{Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
				Body: "done", Events: []string{"download_completed"},
			}}},
		},
	}

	if ok := organizeTorrent(hash, backend, files.NewLibrarian(files.NewOSFileSystem()), fm, fm.configs); !ok {
		t.Fatal("organizeTorrent deveria ter sucesso")
	}
	if len(fm.upserted) != 1 {
		t.Errorf("o marcador LibraryPaths deve ser gravado nos registros novos, upserts = %d", len(fm.upserted))
	}
	select {
	case body := <-events:
		t.Errorf("nenhum webhook deveria sair para um torrent já organizado, obteve %q", body)
	case <-time.After(300 * time.Millisecond):
	}
}
