package daemon

import (
	"fmt"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
)

// appendStandaloneAnimes junta os animes avulsos ao universo do passe, e e chamada DEPOIS do
// DedupeByMedia (ver searchAnilist).
//
// Faz duas coisas, nesta ordem:
//
//  1. Remocao automatica: todo id avulso que ja esta na lista processada sai do arquivo. A
//     entrada real ja vencia o dedupe, entao o que o passe baixa nao muda — o que muda e que o
//     arquivo para de crescer, o chip "Avulso" some da tela e o is_standalone deixa de mentir
//     sobre a origem daquele anime.
//
//     Consequencia aceita: se depois disso o usuario tirar o anime da lista da AniList, o
//     registro avulso ja foi consumido e o passe deixa de cobri-lo — com delete_watched_episodes
//     ligado os episodios dele caem em identifyEpisodesNotInWatching e saem do disco, o mesmo
//     caminho de qualquer anime tirado da lista. A alternativa (distinguir "avulso ativo" de
//     "avulso dormente") custaria estado novo no arquivo, no AnimeInfo e na tela para cobrir um
//     caminho que quem usa avulso nao percorre. Quem quiser o anime de volta o adiciona outra vez.
//
//  2. Append: cada avulso ainda ausente entra como o MediaList sintetico de GetMediaByIDs,
//     buscados todos numa query so.
//     Nao ha filtro de media status aqui de proposito — quem pediu o anime a mao quer que ele
//     seja acompanhado tambem enquanto e NOT_YET_RELEASED, que e metade do sentido da feature.
func appendStandaloneAnimes(fileManager FileManagerInterface, merged []anilist.MediaList, standaloneIDs []int) []anilist.MediaList {
	if len(standaloneIDs) == 0 {
		return merged
	}

	inList := make(map[int]bool, len(merged))
	for _, ml := range merged {
		inList[ml.Media.Id] = true
	}

	pending := make([]int, 0, len(standaloneIDs))
	for _, mediaID := range standaloneIDs {
		if !inList[mediaID] {
			pending = append(pending, mediaID)
			continue
		}
		if err := fileManager.RemoveStandaloneAnime(mediaID); err != nil {
			logger.Logger.Warn().Err(err).Int("media_id", mediaID).
				Msg("Failed to drop the standalone record of an anime now tracked by a list")
			continue
		}
		logger.Logger.Info().Int("media_id", mediaID).
			Msg("Standalone anime is now in an AniList list; dropped its standalone record")
	}
	if len(pending) == 0 {
		return merged
	}

	medias, err := anilist.GetMediaByIDs(pending, anilist.PriorityCritical)
	if err != nil {
		logger.Logger.Warn().Err(err).
			Msg("Failed to fetch standalone animes from AniList; skipping the missing ones this pass")
	}

	for _, mediaID := range pending {
		ml, ok := medias[mediaID]
		if !ok {
			continue // ja avisado pelo erro do lote
		}
		if ml == nil {
			logger.Logger.Warn().Int("media_id", mediaID).
				Msg("AniList does not know this standalone media id; skipping it this pass")
			continue
		}

		merged = append(merged, *withStandaloneProgress(fileManager, ml))
		inList[mediaID] = true
	}

	return merged
}

// withStandaloneProgress da ao MediaList sintetico do avulso o progresso salvo em AnimeSettings.
//
// E a alternativa a um caminho paralelo: com o dado no lugar, shouldSkipEpisode,
// firstEpisodeToConsider, buildWatchedKeepSet, a poda de assistidos e EpisodesWatched funcionam em
// avulso sem nenhum "if isStandalone". Falha de leitura vale como progresso 0 — que e o
// comportamento anterior a este campo.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}

// DownloadStandaloneAnime baixa os episodios ja no ar de um anime avulso na hora em que ele e
// adicionado, sem esperar o proximo ciclo. Devolve quantos episodios entraram.
//
// Faz exatamente tres coisas: garante a sessao de torrents, processa os episodios daquele anime
// e salva os novos registros.
//
// NAO PODE chamar handleSavedEpisodes. Ela chama identifyEpisodesNotInWatching, que compara
// TODOS os episodios salvos contra os checkedEpisodes recebidos e devolve os nao cobertos para
// deleção: com os episodios de um unico anime na mao e delete_watched_episodes ligado, isso
// apagaria o resto da biblioteca inteira.
//
// Tambem nao chama deleteEpisodesByStatus nem DeleteEmptyFolders — adicionar um anime nao e
// ocasiao para apagar nada.
func DownloadStandaloneAnime(fm FileManagerInterface, backend torrents.TorrentBackend, configs *files.Config, mediaID int) (int, error) {
	if _, err := backend.Ensure(configs.DownloadPath()); err != nil {
		return 0, err
	}

	anime, err := anilist.GetMediaByID(mediaID, anilist.PriorityCritical)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch anime %d from AniList: %w", mediaID, err)
	}
	if anime == nil {
		return 0, fmt.Errorf("anime %d does not exist on AniList", mediaID)
	}
	anime = withStandaloneProgress(fm, anime)

	savedEpisodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return 0, fmt.Errorf("failed to load saved episodes: %w", err)
	}

	blockedEpisodes, err := fm.LoadBlockedEpisodes()
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load blocked episodes, continuing without block list")
		blockedEpisodes = nil
	}
	blockedMap := make(map[files.EpisodeKey]bool, len(blockedEpisodes))
	for _, k := range blockedEpisodes {
		blockedMap[k] = true
	}

	// Mesma posse por cobertura do passe: adicionar um avulso cuja season ja esta em disco
	// (baixada sob o cour anterior) nao deve rebaixar nada (coverage.go).
	seriesIndex := resolveSeriesIndex([]int{mediaID}, savedEpisodes)

	result := processAnimeEpisodes(configs, backend, *anime, backend.List(), savedEpisodes, seriesIndex, blockedMap, defaultNyaaSearcher())

	saveEpisodesToFile(fm, result.newEpisodes)

	return len(result.newEpisodes), nil
}
