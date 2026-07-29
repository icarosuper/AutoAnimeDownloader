package daemon

import (
	"fmt"
	"path/filepath"
	"strings"

	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/torrents"
)

// MigrateSavePath converte uma instalacao que ainda tem save_path configurado para o modelo
// de pasta unica: os dados dos torrents sao movidos para <completed_anime_path>/.torrents
// e o campo legado e zerado.
//
// Por que MOVER e nao so repontar: a rain resolve o diretorio de cada torrent como
// filepath.Join(DataDir, torrentID) em tempo de execucao (session_storage.go:29) — o caminho
// nao vai para o resume data. Trocar o save path sem mover faria todo torrent existente
// apontar para um diretorio vazio, reverificar, achar nada e rebaixar tudo.
//
// Por que o rename e seguro: o probe de hardlink sempre exigiu que save path e biblioteca
// estivessem no mesmo volume, entao qualquer config que funcionava tem origem e destino no
// mesmo filesystem. Rename preserva o inode: o torrent segue semeando dos mesmos bytes e os
// hardlinks ja criados na biblioteca continuam validos.
//
// E idempotente: chamada no boot e no topo do passe de verificacao.
func MigrateSavePath(fs files.FileSystem, fm FileManagerInterface, backend torrents.TorrentBackend) error {
	configs, err := fm.LoadConfigs()
	if err != nil {
		return fmt.Errorf("migration: failed to load configs: %w", err)
	}
	if configs == nil || configs.SavePath == "" {
		return nil // nada a migrar
	}
	if configs.CompletedAnimePath == "" {
		// Config incompleta: sem biblioteca nao ha destino. O passe de verificacao tenta
		// de novo depois que o usuario salvar a configuracao.
		return nil
	}

	dest := configs.DownloadPath()
	oldSavePath := configs.SavePath

	if oldSavePath == dest {
		configs.SavePath = ""
		if err := fm.SaveConfigs(configs); err != nil {
			return fmt.Errorf("migration: failed to clear the legacy save path: %w", err)
		}
		logger.Logger.Info().Msg("Migration: legacy save path already matched the derived download path; cleared the field")
		return nil
	}

	var dataDirs []string
	if _, statErr := fs.Stat(oldSavePath); statErr != nil {
		// A pasta antiga ja nao existe: uma execucao anterior ja moveu tudo (ou nao havia
		// nada), so a limpeza do campo no config falhou. Nao ha motivo para abrir uma sessao
		// da rain num diretorio que nao existe mais — trata como "nada a mover" e segue
		// direto para zerar o SavePath.
		logger.Logger.Info().Str("old_save_path", oldSavePath).
			Msg("Migration: the legacy save path no longer exists; nothing to move")
	} else {
		if backend == nil {
			return fmt.Errorf("migration: torrent backend not initialized")
		}

		// Abrir a sessao no caminho ANTIGO e listar e o que torna a migracao precisa: move
		// exatamente os diretorios que sao torrents, e nada que o usuario tenha deixado no
		// save path.
		if _, err := backend.Ensure(oldSavePath); err != nil {
			return fmt.Errorf("migration: failed to open the torrent session at the old save path %s: %w", oldSavePath, err)
		}
		for _, t := range backend.List() {
			if t.DataDir != "" {
				dataDirs = append(dataDirs, t.DataDir)
			}
		}
		if err := backend.Close(); err != nil {
			logger.Logger.Warn().Err(err).Msg("Migration: error closing the temporary torrent session")
		}
	}

	if err := fs.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("migration: cannot create the download folder %s: %w", dest, err)
	}

	moved := 0
	for _, dir := range dataDirs {
		if isAncestorOrEqual(dir, dest) {
			// Caso patologico: mover um diretorio para dentro dele mesmo. Nao acontece
			// com DataDirs normais (<save>/<uuid>), mas o layout default do Docker
			// aninha a biblioteca dentro do save path, entao a guarda e explicita.
			logger.Logger.Warn().Str("data_dir", dir).Str("dest", dest).
				Msg("Migration: skipping a torrent directory that contains the destination")
			continue
		}
		target := filepath.Join(dest, filepath.Base(dir))
		if _, err := fs.Stat(target); err == nil {
			continue // ja migrado numa execucao anterior
		}
		if err := fs.Rename(dir, target); err != nil {
			return fmt.Errorf("migration: failed to move %s to %s: %w", dir, target, err)
		}
		moved++
	}

	// O marcador de raiz vai junto com os dados. Sem isso a migracao pareceria uma pasta
	// trocada para o proximo Ensure, que zeraria os LibraryPaths de uma biblioteca cujos
	// hardlinks o rename acabou de preservar.
	marker := filepath.Join(oldSavePath, torrents.RootMarkerName)
	if _, err := fs.Stat(marker); err == nil {
		if err := fs.Rename(marker, filepath.Join(dest, torrents.RootMarkerName)); err != nil {
			logger.Logger.Warn().Err(err).Msg("Migration: failed to move the download root marker")
		}
	}

	configs.SavePath = ""
	if err := fm.SaveConfigs(configs); err != nil {
		return fmt.Errorf("migration: moved %d torrent folders but failed to clear the legacy save path: %w", moved, err)
	}

	// So tem efeito se a pasta antiga ficou vazia. Sobras do usuario ficam onde estao.
	if err := fs.Remove(oldSavePath); err != nil {
		logger.Logger.Info().Str("old_save_path", oldSavePath).
			Msg("Migration: the old save path still has files in it and was left in place")
	}

	logger.Logger.Info().
		Str("old_save_path", oldSavePath).
		Str("new_download_path", dest).
		Int("moved", moved).
		Msg("Migration: moved the download folder into the library; seeding and library hardlinks were preserved")
	return nil
}

// isAncestorOrEqual reporta se dir e o proprio child ou um diretorio acima dele.
func isAncestorOrEqual(dir, child string) bool {
	dirClean := filepath.Clean(dir)
	childClean := filepath.Clean(child)
	if dirClean == childClean {
		return true
	}
	return strings.HasPrefix(childClean, dirClean+string(filepath.Separator))
}
