package files

import (
	"AutoAnimeDownloader/src/internal/logger"
	"AutoAnimeDownloader/src/internal/nyaa"

	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Librarian organizes completed torrent files into the Jellyfin library by creating
// hardlinks. The original files stay in place (kept seeding); the library holds a second
// name pointing at the same bytes, so no space is duplicated.
type Librarian interface {
	// Organize creates hardlinks for the completed video files of a torrent in the
	// library. With RenameJellyfin it uses the Jellyfin name ("Anime - E05.mkv") — from
	// the record for a single episode, from each file's own name for a batch; a file
	// whose number can't be read (and everything without the flag) keeps the raw name.
	// It returns the absolute paths of the library links it created (or that already
	// existed) so the caller can record them for later removal. It is idempotent: a
	// destination that is already the same file (same inode) is reported and skipped.
	// A destination holding a *different* file is replaced by the new hardlink, which
	// is what the redownload/replace flows want.
	Organize(req OrganizeRequest) ([]string, error)
	// RemoveFromLibrary deletes a single library hardlink. A missing file is not an error.
	RemoveFromLibrary(path string) error
	// ProbePath valida, no save da config e a cada passe de verificacao, que a biblioteca
	// suporta hardlinks. O cheque de volume cruzado deixou de ser necessario (o diretorio
	// de download e derivado da biblioteca, entao estao sempre no mesmo filesystem), mas
	// existem filesystems sem hardlink nenhum: exFAT, FAT32, alguns mounts SMB/NFS. Usa a
	// mesma funcao de link que Organize usa, entao nunca discorda dele. Tambem cria o
	// diretorio de download e o marcador .ignore.
	ProbePath(completedPath string) error
}

// OrganizeRequest describes one torrent to organize into the library.
type OrganizeRequest struct {
	// TorrentDataDir is the on-disk root of the torrent's content (<DataDir>/<id>).
	TorrentDataDir string
	AnimeName      string
	// AnimeID e o id de MIDIA da AniList; vira o <uniqueid> do tvshow.nfo para o Jellyfin
	// casar pelo id em vez de adivinhar pelo titulo da pasta. Zero = pula o nfo.
	AnimeID       int
	CompletedPath string
	// EpisodeNumber is used for the Jellyfin name; required when RenameJellyfin is set
	// for a single episode.
	EpisodeNumber *int
	// IsBatch marks multi-episode/movie torrents: the episode number comes from each
	// file's own name instead of EpisodeNumber.
	IsBatch bool
	// RenameJellyfin enables the "Anime - E05.ext" naming.
	RenameJellyfin bool
}

type organizer struct {
	fs   FileSystem
	link func(oldname, newname string) error
}

// NewLibrarian returns a Librarian backed by the given FileSystem. The link function
// defaults to fs.Link; both Organize and ProbePath use it, so they never diverge.
func NewLibrarian(fs FileSystem) *organizer {
	return &organizer{fs: fs, link: fs.Link}
}

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".webm": true, ".flv": true, ".wmv": true, ".ts": true, ".mpg": true,
	".mpeg": true, ".ogm": true,
}

func isVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(name))]
}

// sanitizeName strips filesystem-invalid characters. O marcador de season e MANTIDO: uma
// pasta por entrada da AniList (decisions.md #45).
func sanitizeName(name string) string {
	sanitized := stripInvalidChars(name)
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.ReplaceAll(sanitized, "  ", " ")
	return sanitized
}

func stripInvalidChars(name string) string {
	invalidChars := []string{":", "<", ">", "|", "?", "*", "\"", "\\", "/"}
	sanitized := name
	for _, char := range invalidChars {
		sanitized = strings.ReplaceAll(sanitized, char, "")
	}
	return sanitized
}

// jellyfinName returns "Anime - E05.ext".
func jellyfinName(animeName string, episodeNumber int, ext string) string {
	return fmt.Sprintf("%s - E%02d%s", sanitizeName(animeName), episodeNumber, ext)
}

func (o *organizer) Organize(req OrganizeRequest) ([]string, error) {
	// Guard first: with an empty CompletedPath, filepath.Join below yields a relative
	// path and MkdirAll would create the library folder in the process' working dir.
	if req.CompletedPath == "" {
		return nil, fmt.Errorf("completed anime path is not configured")
	}

	videoFiles, err := o.collectVideoFiles(req.TorrentDataDir)
	if err != nil {
		return nil, err
	}
	if len(videoFiles) == 0 {
		return nil, fmt.Errorf("no video files found in %s", req.TorrentDataDir)
	}

	destDir := filepath.Join(req.CompletedPath, sanitizeName(req.AnimeName))

	// Track whether we created destDir, so we can clean it up on a cross-device failure
	// without leaving an orphan folder in the library.
	dirExisted := true
	if _, statErr := o.fs.Stat(destDir); statErr != nil {
		dirExisted = false
	}
	if err := o.fs.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create library folder %s: %w", destDir, err)
	}

	// Jellyfin naming for a genuine single episode with one video file. EpisodeNumber == 0
	// means "missing data" (AniList numbers episodes from 1), so we fall back to the raw
	// filename instead of colliding every episode on "Anime - E00".
	singleJellyfin := !req.IsBatch && req.RenameJellyfin && req.EpisodeNumber != nil &&
		*req.EpisodeNumber > 0 && len(videoFiles) == 1

	used := make(map[string]bool, len(videoFiles))
	var created []string
	for _, rel := range videoFiles {
		src := filepath.Join(req.TorrentDataDir, rel)

		destName := filepath.Base(rel)
		switch {
		case singleJellyfin:
			destName = jellyfinName(req.AnimeName, *req.EpisodeNumber, filepath.Ext(rel))
		case req.IsBatch && req.RenameJellyfin:
			// Pack: o numero sai do proprio nome do arquivo, para os episodios do pack se
			// misturarem na pasta com os avulsos em vez de manter o nome cru do fansub.
			// Sem numero legivel (NCOP/NCED, extra, filme) ou com colisao entre dois
			// arquivos do mesmo pack, fica o nome cru — que e unico dentro do torrent.
			if n := nyaa.ExtractEpisodeNumber(destName); n != nil {
				if jf := jellyfinName(req.AnimeName, *n, filepath.Ext(rel)); !used[jf] {
					destName = jf
				}
			}
		}
		used[destName] = true
		dest := filepath.Join(destDir, destName)

		if destInfo, statErr := o.fs.Stat(dest); statErr == nil {
			srcInfo, srcErr := o.fs.Stat(src)
			if srcErr != nil {
				return nil, fmt.Errorf("failed to stat source %s: %w", src, srcErr)
			}
			if os.SameFile(srcInfo, destInfo) {
				// Idempotent: this exact file is already linked (reconciliation/retry).
				created = append(created, dest)
				continue
			}
			// Different bytes under the same name (redownload/replace): the user asked
			// for the swap, so the new file wins.
			logger.Logger.Info().
				Str("source", src).
				Str("destination", dest).
				Msg("Replacing existing library file with the newly downloaded one")
			if err := o.fs.Remove(dest); err != nil {
				o.cleanupIfEmpty(destDir, dirExisted)
				return nil, fmt.Errorf("failed to replace existing library file %s: %w", dest, err)
			}
		}

		if err := o.link(src, dest); err != nil {
			o.cleanupIfEmpty(destDir, dirExisted)
			if isCrossDevice(err) {
				return nil, fmt.Errorf("cannot hardlink %s -> %s: save path and completed path must be on the same volume: %w", src, dest, err)
			}
			return nil, fmt.Errorf("failed to hardlink %s -> %s: %w", src, dest, err)
		}
		created = append(created, dest)
	}

	// Depois dos links: se falhar antes, cleanupIfEmpty nao conseguiria remover a pasta.
	o.writeShowNFO(destDir, req.AnimeName, req.AnimeID)

	return created, nil
}

type nfoUniqueID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

type nfoTVShow struct {
	XMLName  xml.Name    `xml:"tvshow"`
	Title    string      `xml:"title"`
	UniqueID nfoUniqueID `xml:"uniqueid"`
}

// writeShowNFO escreve o tvshow.nfo com o id da AniList para o Jellyfin (plugin AniList)
// casar pelo id. Nao sobrescreve um nfo existente — o usuario pode ter ajustado o match a
// mao. Falha aqui nao invalida os hardlinks, entao so loga. Retorna true se escreveu.
func (o *organizer) writeShowNFO(destDir, animeName string, animeID int) bool {
	if animeID <= 0 {
		return false
	}
	path := filepath.Join(destDir, "tvshow.nfo")
	if _, err := o.fs.Stat(path); err == nil {
		return false
	}

	data, err := xml.MarshalIndent(nfoTVShow{
		Title:    animeName,
		// "AniList" com essa capitalizacao e o valor de ProviderNames.AniList no
		// jellyfin-plugin-anilist. O ProviderIds do Jellyfin e OrdinalIgnoreCase, entao
		// minusculo tambem casaria — escrevemos igual ao provider para nao depender disso.
		UniqueID: nfoUniqueID{Type: "AniList", Default: true, Value: strconv.Itoa(animeID)},
	}, "", "  ")
	if err != nil {
		logger.Logger.Warn().Err(err).Str("path", path).Msg("Failed to build tvshow.nfo")
		return false
	}
	data = append([]byte(xml.Header), append(data, '\n')...)

	if err := o.fs.WriteFile(path, data, 0644); err != nil {
		logger.Logger.Warn().Err(err).Str("path", path).Msg("Failed to write tvshow.nfo")
		return false
	}
	return true
}

// BackfillShowNFOs escreve o tvshow.nfo das pastas que ja estavam na biblioteca antes de o
// nfo existir: Organize so roda para episodio novo (sai cedo quando LibraryPaths ja esta
// preenchido), entao sem isso um anime que ja terminou nunca ganharia o arquivo. A pasta sai
// de LibraryPaths, nao do nome do anime, para casar com o que foi realmente criado em disco
// (sanitizacao, renomeacao manual). Uma pasta por anime; pasta que sumiu do disco e pulada.
//
// SO PODE RODAR COM OS AnimeID JA MIGRADOS para id de midia (decisions.md #43): como o nfo
// nunca e sobrescrito, gravar um id de entrada aqui seria permanente.
func (o *organizer) BackfillShowNFOs(episodes []EpisodeStruct) {
	seen := make(map[string]bool)
	written := 0
	for _, ep := range episodes {
		if ep.AnimeID <= 0 || len(ep.LibraryPaths) == 0 {
			continue
		}
		dir := filepath.Dir(ep.LibraryPaths[0])
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if _, err := o.fs.Stat(dir); err != nil {
			continue // pasta removida da biblioteca por fora
		}
		if o.writeShowNFO(dir, ep.AnimeName, ep.AnimeID) {
			written++
		}
	}
	if written > 0 {
		logger.Logger.Info().Int("count", written).Msg("Backfilled tvshow.nfo for existing library folders")
	}
}

func (o *organizer) RemoveFromLibrary(path string) error {
	if path == "" {
		return nil
	}
	if err := o.fs.Remove(path); err != nil {
		if _, statErr := o.fs.Stat(path); statErr != nil {
			// Already gone — not an error.
			return nil
		}
		return err
	}
	return nil
}

func (o *organizer) ProbePath(completedPath string) error {
	if completedPath == "" {
		return fmt.Errorf("completed anime path must be set")
	}
	if err := o.fs.MkdirAll(completedPath, 0755); err != nil {
		return fmt.Errorf("cannot access completed path %s: %w", completedPath, err)
	}

	downloadPath := filepath.Join(completedPath, downloadDirName)
	if err := o.fs.MkdirAll(downloadPath, 0755); err != nil {
		return fmt.Errorf("cannot create download folder %s: %w", downloadPath, err)
	}

	// O prefixo com ponto esconde a pasta do scanner do Jellyfin no Linux; o .ignore cobre
	// as plataformas onde o ponto nao marca oculto. As duas defesas juntas, porque a
	// pasta de download agora vive dentro da pasta que o Jellyfin varre.
	ignorePath := filepath.Join(downloadPath, ".ignore")
	if _, err := o.fs.Stat(ignorePath); err != nil {
		if err := o.fs.WriteFile(ignorePath, nil, 0644); err != nil {
			return fmt.Errorf("cannot write ignore marker %s: %w", ignorePath, err)
		}
	}

	probeSrc := filepath.Join(downloadPath, ".aad_link_probe")
	probeDst := filepath.Join(completedPath, ".aad_link_probe")

	// Limpa sobras de uma sonda anterior.
	_ = o.fs.Remove(probeSrc)
	_ = o.fs.Remove(probeDst)

	if err := o.fs.WriteFile(probeSrc, []byte("probe"), 0644); err != nil {
		return fmt.Errorf("cannot write to download path %s: %w", downloadPath, err)
	}
	defer func() { _ = o.fs.Remove(probeSrc) }()

	if err := o.link(probeSrc, probeDst); err != nil {
		return fmt.Errorf("this filesystem does not support hardlinks, which the library requires: %w", err)
	}
	_ = o.fs.Remove(probeDst)

	return nil
}

// collectVideoFiles returns the video-file paths under root, relative to root.
func (o *organizer) collectVideoFiles(root string) ([]string, error) {
	var out []string
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		entries, err := o.fs.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childRel := filepath.Join(rel, e.Name())
			childAbs := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if err := walk(childAbs, childRel); err != nil {
					return err
				}
				continue
			}
			if isVideoFile(e.Name()) {
				out = append(out, childRel)
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (o *organizer) cleanupIfEmpty(dir string, dirExisted bool) {
	if dirExisted {
		return
	}
	// os.Remove only succeeds on an empty dir; a non-empty pre-existing dir is left alone.
	_ = o.fs.Remove(dir)
}
