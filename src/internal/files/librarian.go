package files

import (
	"AutoAnimeDownloader/src/internal/logger"

	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Librarian organizes completed torrent files into the Jellyfin library by creating
// hardlinks. The original files stay in place (kept seeding); the library holds a second
// name pointing at the same bytes, so no space is duplicated.
type Librarian interface {
	// Organize creates hardlinks for the completed video files of a torrent in the
	// library. For a single episode with RenameJellyfin it uses the Jellyfin name
	// ("Anime - E05.mkv"); for a batch (or without the flag) it keeps the raw filename.
	// It returns the absolute paths of the library links it created (or that already
	// existed) so the caller can record them for later removal. It is idempotent: a
	// destination that is already the same file (same inode) is reported and skipped.
	// A destination holding a *different* file is replaced by the new hardlink, which
	// is what the redownload/replace flows want.
	Organize(req OrganizeRequest) ([]string, error)
	// RemoveFromLibrary deletes a single library hardlink. A missing file is not an error.
	RemoveFromLibrary(path string) error
	// ProbePaths validates, at config-save time, that savePath and completedPath live on
	// the same volume (hardlinks cannot cross filesystems). It uses the exact same link
	// function the runtime uses, so it can never disagree with Organize.
	ProbePaths(savePath, completedPath string) error
}

// OrganizeRequest describes one torrent to organize into the library.
type OrganizeRequest struct {
	// TorrentDataDir is the on-disk root of the torrent's content (<DataDir>/<id>).
	TorrentDataDir string
	AnimeName      string
	CompletedPath  string
	// EpisodeNumber is used for the Jellyfin name; required when RenameJellyfin is set
	// for a single episode.
	EpisodeNumber *int
	// IsBatch marks multi-episode/movie torrents: hardlink raw, never Jellyfin-renamed.
	IsBatch bool
	// RenameJellyfin enables the "Anime - E05.ext" naming for single episodes.
	RenameJellyfin bool
}

type organizer struct {
	fs   FileSystem
	link func(oldname, newname string) error
}

// NewLibrarian returns a Librarian backed by the given FileSystem. The link function
// defaults to fs.Link; both Organize and ProbePaths use it, so they never diverge.
func NewLibrarian(fs FileSystem) *organizer {
	return &organizer{fs: fs, link: fs.Link}
}

var seasonPattern = regexp.MustCompile(`(?i)\s+(?:season\s*\d+|s\s*\d+|\d+(?:st|nd|rd|th)\s+season|cour\s*\d+)`)

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true,
	".webm": true, ".flv": true, ".wmv": true, ".ts": true, ".mpg": true,
	".mpeg": true, ".ogm": true,
}

func isVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(name))]
}

// sanitizeFolderName strips filesystem-invalid characters and season/cour markers.
func sanitizeFolderName(name string) string {
	sanitized := stripInvalidChars(name)
	sanitized = seasonPattern.ReplaceAllString(sanitized, "")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.ReplaceAll(sanitized, "  ", " ")
	return sanitized
}

// sanitizeFileName strips filesystem-invalid characters (keeps season markers).
func sanitizeFileName(name string) string {
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
	return fmt.Sprintf("%s - E%02d%s", sanitizeFileName(animeName), episodeNumber, ext)
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

	destDir := filepath.Join(req.CompletedPath, sanitizeFolderName(req.AnimeName))

	// Track whether we created destDir, so we can clean it up on a cross-device failure
	// without leaving an orphan folder in the library.
	dirExisted := true
	if _, statErr := o.fs.Stat(destDir); statErr != nil {
		dirExisted = false
	}
	if err := o.fs.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create library folder %s: %w", destDir, err)
	}

	// Jellyfin naming only applies to a genuine single episode with one video file.
	// EpisodeNumber == 0 means "missing data" (AniList numbers episodes from 1), so we
	// fall back to the raw filename instead of colliding every episode on "Anime - E00".
	useJellyfin := !req.IsBatch && req.RenameJellyfin && req.EpisodeNumber != nil &&
		*req.EpisodeNumber > 0 && len(videoFiles) == 1

	var created []string
	for _, rel := range videoFiles {
		src := filepath.Join(req.TorrentDataDir, rel)

		var destName string
		if useJellyfin {
			destName = jellyfinName(req.AnimeName, *req.EpisodeNumber, filepath.Ext(rel))
		} else {
			destName = filepath.Base(rel)
		}
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

	return created, nil
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

func (o *organizer) ProbePaths(savePath, completedPath string) error {
	if savePath == "" || completedPath == "" {
		return fmt.Errorf("save path and completed path must both be set")
	}
	if err := o.fs.MkdirAll(savePath, 0755); err != nil {
		return fmt.Errorf("cannot access save path %s: %w", savePath, err)
	}
	if err := o.fs.MkdirAll(completedPath, 0755); err != nil {
		return fmt.Errorf("cannot access completed path %s: %w", completedPath, err)
	}

	probeSrc := filepath.Join(savePath, ".aad_link_probe")
	probeDst := filepath.Join(completedPath, ".aad_link_probe")

	// Clean up any leftovers from a previous probe.
	_ = o.fs.Remove(probeSrc)
	_ = o.fs.Remove(probeDst)

	if err := o.fs.WriteFile(probeSrc, []byte("probe"), 0644); err != nil {
		return fmt.Errorf("cannot write to save path %s: %w", savePath, err)
	}
	defer func() { _ = o.fs.Remove(probeSrc) }()

	if err := o.link(probeSrc, probeDst); err != nil {
		if isCrossDevice(err) {
			return fmt.Errorf("save path and completed path are on different volumes; hardlinks require the same filesystem")
		}
		return fmt.Errorf("failed to verify hardlink support between save and completed paths: %w", err)
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
