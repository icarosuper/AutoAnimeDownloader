package daemon

import (
	"fmt"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
)

// migrateAnimeIDsThrottle espaca as consultas da migracao. A AniList degrada para ~30
// requisicoes por minuto sob carga e o cliente nao tem retry: uma migracao que dispara 50
// consultas em rajada volta com 429, aborta, e retenta a rajada no passe seguinte.
var migrateAnimeIDsThrottle = 250 * time.Millisecond

// MigrateAnimeIDsToMedia converte o AnimeID persistido de id de ENTRADA (MediaList, que e por
// conta) para id de MIDIA — ver decisions.md #43. Enquanto nao rodar, os registros em disco
// estao chaveados de um jeito que o resto do codigo ja nao entende, entao o chamador deve
// ABORTAR o passe quando ela falhar: seguir em frente faria todo anime parecer nao-baixado e
// o daemon rebaixaria a biblioteca inteira.
//
// E idempotente pelo campo AnimeIDsAreMediaIDs do config, e so escreve depois de resolver
// TODAS as entradas: uma falha de rede no meio nao deixa metade dos registros convertidos.
//
// Entradas que ja nao existem na AniList (o usuario removeu o anime da lista) nao tem como ser
// resolvidas; esses registros ficam com o id antigo e sao apenas logados. Os episodios
// continuam em disco e listados — so nao voltam a casar com a AniList se o anime for readicionado.
func MigrateAnimeIDsToMedia(fm FileManagerInterface) error {
	configs, err := fm.LoadConfigs()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load configs: %w", err)
	}
	if configs.AnimeIDsAreMediaIDs {
		return nil
	}

	episodes, err := fm.LoadSavedEpisodes()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load saved episodes: %w", err)
	}
	settings, err := fm.LoadAllAnimeSettings()
	if err != nil {
		return fmt.Errorf("anime id migration: failed to load anime settings: %w", err)
	}

	oldIDs := make(map[int]bool)
	for _, ep := range episodes {
		if ep.AnimeID != 0 {
			oldIDs[ep.AnimeID] = true
		}
	}
	for animeID := range settings {
		if animeID != 0 {
			oldIDs[animeID] = true
		}
	}

	mediaByEntry := make(map[int]int, len(oldIDs))
	var unresolved []int
	for entryID := range oldIDs {
		mediaID, err := anilist.GetMediaIDForEntry(entryID)
		if err != nil {
			// Nada foi escrito ainda: o passe seguinte recomeca a migracao inteira.
			return fmt.Errorf("anime id migration: failed to resolve entry %d: %w", entryID, err)
		}
		if mediaID == 0 {
			unresolved = append(unresolved, entryID)
			continue
		}
		mediaByEntry[entryID] = mediaID
		time.Sleep(migrateAnimeIDsThrottle)
	}

	var updated []files.EpisodeStruct
	for _, ep := range episodes {
		if mediaID, ok := mediaByEntry[ep.AnimeID]; ok && mediaID != ep.AnimeID {
			ep.AnimeID = mediaID
			updated = append(updated, ep)
		}
	}
	if err := fm.UpsertEpisodes(updated); err != nil {
		return fmt.Errorf("anime id migration: failed to rewrite episodes: %w", err)
	}

	// As chaves antigas ficam no arquivo de settings: nao ha remocao na interface e uma entrada
	// orfa de settings nao atrapalha nada. Copiar e o que importa.
	for entryID, s := range settings {
		mediaID, ok := mediaByEntry[entryID]
		if !ok || mediaID == entryID {
			continue
		}
		if err := fm.SaveAnimeSettings(mediaID, s); err != nil {
			return fmt.Errorf("anime id migration: failed to rewrite settings for anime %d: %w", mediaID, err)
		}
	}

	configs.AnimeIDsAreMediaIDs = true
	if err := fm.SaveConfigs(configs); err != nil {
		return fmt.Errorf("anime id migration: failed to record completion: %w", err)
	}

	logger.Logger.Info().
		Int("entries_resolved", len(mediaByEntry)).
		Int("episodes_rewritten", len(updated)).
		Ints("entries_no_longer_on_anilist", unresolved).
		Msg("Migration: anime IDs are now AniList media IDs")

	return nil
}
