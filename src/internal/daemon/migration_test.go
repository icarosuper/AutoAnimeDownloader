package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// animeIDMigrationFM guarda episodios e settings em memoria para que o teste veja o que a
// migracao reescreveu.
type animeIDMigrationFM struct {
	mockFileManagerForEpisodes
	configs  *files.Config
	episodes []files.EpisodeStruct
	settings map[int]files.AnimeSettings
}

func (m *animeIDMigrationFM) LoadConfigs() (*files.Config, error) { return m.configs, nil }
func (m *animeIDMigrationFM) SaveConfigs(c *files.Config) error   { m.configs = c; return nil }
func (m *animeIDMigrationFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	return m.episodes, nil
}
func (m *animeIDMigrationFM) UpsertEpisodes(eps []files.EpisodeStruct) error {
	for _, up := range eps {
		for i := range m.episodes {
			if m.episodes[i].EpisodeNumber == up.EpisodeNumber {
				m.episodes[i] = up
			}
		}
	}
	return nil
}
func (m *animeIDMigrationFM) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return m.settings, nil
}
func (m *animeIDMigrationFM) SaveAnimeSettings(id int, s files.AnimeSettings) error {
	if m.settings == nil {
		m.settings = map[int]files.AnimeSettings{}
	}
	m.settings[id] = s
	return nil
}

// mockEntryToMedia responde a query de entrada->midia com o mapa dado. Uma entrada ausente
// vira 404, que e o que a AniList devolve para uma entrada que o usuario apagou.
func mockEntryToMedia(t *testing.T, byEntry map[int]int) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Variables struct {
				MediaListId int `json:"mediaListId"`
			} `json:"variables"`
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("corpo inesperado: %s", body)
		}
		mediaID, ok := byEntry[payload.Variables.MediaListId]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("Not Found"))}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"MediaList":{"media":{"id":%d}}}}`, mediaID))),
		}, nil
	})
}

func TestMigrateAnimeIDsToMedia_RewritesEpisodesAndSettings(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer mockEntryToMedia(t, map[int]int{488911345: 194829, 583631757: 194829})()

	fm := &animeIDMigrationFM{
		configs: &files.Config{},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 583631757},
			{EpisodeNumber: 2, AnimeID: 488911345},
		},
		settings: map[int]files.AnimeSettings{583631757: {Progress: 42}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migracao falhou: %v", err)
	}

	// As duas entradas apontam para a MESMA midia: e exatamente o caso que duplicava o anime.
	for _, ep := range fm.episodes {
		if ep.AnimeID != 194829 {
			t.Errorf("episodio %d deveria estar com o id da midia, veio %d", ep.EpisodeNumber, ep.AnimeID)
		}
	}
	if fm.settings[194829].Progress != 42 {
		t.Errorf("as settings deveriam ter sido copiadas para o id da midia, veio %+v", fm.settings)
	}
	if !fm.configs.AnimeIDsAreMediaIDs {
		t.Error("a migracao deveria marcar a conclusao no config")
	}
}

// Idempotencia: com a marca ligada nao pode haver nem consulta nem reescrita — rodar de novo
// interpretaria os ids de MIDIA como ids de entrada e embaralharia tudo.
func TestMigrateAnimeIDsToMedia_SkipsWhenAlreadyMigrated(t *testing.T) {
	called := false
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})()

	fm := &animeIDMigrationFM{
		configs:  &files.Config{AnimeIDsAreMediaIDs: true},
		episodes: []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 194829}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migracao falhou: %v", err)
	}
	if called {
		t.Error("nao deveria consultar a AniList com a migracao ja concluida")
	}
}

// Falha de rede no meio nao pode deixar metade convertida: nada e escrito e a marca fica
// desligada, para o passe seguinte recomecar do zero.
func TestMigrateAnimeIDsToMedia_FailureWritesNothing(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})()

	fm := &animeIDMigrationFM{
		configs:  &files.Config{},
		episodes: []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 583631757}},
	}

	if err := MigrateAnimeIDsToMedia(fm); err == nil {
		t.Fatal("esperava erro quando a AniList falha")
	}
	if fm.episodes[0].AnimeID != 583631757 {
		t.Errorf("nada deveria ter sido reescrito, veio %d", fm.episodes[0].AnimeID)
	}
	if fm.configs.AnimeIDsAreMediaIDs {
		t.Error("a marca de conclusao nao pode ser ligada apos uma falha")
	}
}

// Entrada apagada da AniList (404) nao trava a migracao: o registro fica com o id antigo e o
// resto converte normalmente.
func TestMigrateAnimeIDsToMedia_UnresolvedEntryDoesNotBlock(t *testing.T) {
	migrateAnimeIDsThrottle = 0
	defer mockEntryToMedia(t, map[int]int{111: 500})()

	fm := &animeIDMigrationFM{
		configs: &files.Config{},
		episodes: []files.EpisodeStruct{
			{EpisodeNumber: 1, AnimeID: 111},
			{EpisodeNumber: 2, AnimeID: 222}, // nao existe mais
		},
	}

	if err := MigrateAnimeIDsToMedia(fm); err != nil {
		t.Fatalf("migracao falhou: %v", err)
	}
	if fm.episodes[0].AnimeID != 500 {
		t.Errorf("entrada resolvivel deveria virar 500, veio %d", fm.episodes[0].AnimeID)
	}
	if fm.episodes[1].AnimeID != 222 {
		t.Errorf("entrada sem correspondencia deve ficar como estava, veio %d", fm.episodes[1].AnimeID)
	}
	if !fm.configs.AnimeIDsAreMediaIDs {
		t.Error("a migracao deveria concluir mesmo com entradas nao resolvidas")
	}
}
