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
	// the record for a single episode, from each file's own name for a batch (translated
	// to the entry's numbering when the pack numbers continuously, see
	// packEpisodeOffset); a file whose number can't be read (and everything without the
	// flag) keeps the raw name.
	// It returns the absolute paths of the library links it created (or that already
	// existed) so the caller can record them for later removal. It is idempotent: a
	// destination that is already the same file (same inode) is reported and skipped.
	// A destination holding a *different* file is replaced by the new hardlink, which
	// is what the redownload/replace flows want.
	Organize(req OrganizeRequest) ([]string, error)
	// RemoveFromLibrary deletes a single library hardlink. A missing file is not an error.
	RemoveFromLibrary(path string) error
	// ProbePath validates, on config save and on every verification pass, that the library
	// supports hardlinks. The cross-volume check is no longer needed (the download
	// directory is derived from the library, so they are always on the same filesystem), but
	// filesystems with no hardlinks at all exist: exFAT, FAT32, some SMB/NFS mounts. Uses the
	// same link function Organize uses, so it never disagrees with it. Also creates the
	// download directory and the .ignore marker.
	ProbePath(completedPath string) error
}

// OrganizeRequest describes one torrent to organize into the library.
type OrganizeRequest struct {
	// TorrentDataDir is the on-disk root of the torrent's content (<DataDir>/<id>).
	TorrentDataDir string
	AnimeName      string
	// AnimeID is the AniList MEDIA id; it becomes the <uniqueid> of tvshow.nfo so Jellyfin
	// matches by id instead of guessing from the folder title. Zero = skip the nfo.
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
	// TotalEpisodes is the episode total of the AniList ENTRY. Used only to detect a pack
	// with continuous numbering (see packEpisodeOffset); zero = unknown, nothing is shifted.
	TotalEpisodes int
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

// The extension list lives in the nyaa package: PackFileRange reads the SAME list of pack
// files before downloading, and two extension lists would diverge.
func isVideoFile(name string) bool {
	return nyaa.IsVideoFile(name)
}

// sanitizeName strips filesystem-invalid characters. The season marker is KEPT: one
// folder per AniList entry (decisions.md #45).
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

// packEpisodeOffset returns how much to subtract from the number read in the file name to
// reach the AniList ENTRY number. A season >= 2 pack with continuous numbering (files 13-24
// for a 12-episode entry) is the case; anything else returns 0.
//
// ponytail: heuristic over the files themselves, without consulting AniList during organize.
// Only fires with unequivocal evidence — EVERY numbered file above the entry total AND at
// least one file per episode, i.e. a complete pack that starts at episode 1 of the season. An
// extra numbered 13 in a 01-12 pack keeps the minimum at 1 and shifts nothing. A PARTIAL pack
// with continuous numbering stays as-is; if that becomes a requirement, the path is to persist
// the offset (the PREQUEL episode count, already computed in daemon.ComputeEpisodeOffset) on
// EpisodeStruct instead of guessing here.
func packEpisodeOffset(numbers []int, totalEpisodes int) int {
	if totalEpisodes <= 0 || len(numbers) < totalEpisodes {
		return 0
	}
	lowest := numbers[0]
	for _, n := range numbers {
		if n < lowest {
			lowest = n
		}
	}
	if lowest <= totalEpisodes {
		return 0
	}
	return lowest - 1
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

	var packOffset int
	if req.IsBatch && req.RenameJellyfin {
		var numbers []int
		for _, rel := range videoFiles {
			if n := nyaa.ExtractEpisodeNumber(filepath.Base(rel)); n != nil {
				numbers = append(numbers, *n)
			}
		}
		packOffset = packEpisodeOffset(numbers, req.TotalEpisodes)
	}

	used := make(map[string]bool, len(videoFiles))
	var created []string
	for _, rel := range videoFiles {
		src := filepath.Join(req.TorrentDataDir, rel)

		destName := filepath.Base(rel)
		switch {
		case singleJellyfin:
			destName = jellyfinName(req.AnimeName, *req.EpisodeNumber, filepath.Ext(rel))
		case req.IsBatch && req.RenameJellyfin:
			// Pack: the number comes from the file's own name, so pack episodes mix in the
			// folder with the singles instead of keeping the fansub's raw name.
			// Without a readable number (NCOP/NCED, extra, movie) or on a collision between
			// two files of the same pack, the raw name stays.
			if n := nyaa.ExtractEpisodeNumber(destName); n != nil {
				if jf := jellyfinName(req.AnimeName, *n-packOffset, filepath.Ext(rel)); !used[jf] {
					destName = jf
				}
			}
		}
		// Two files of the SAME torrent with the same basename (multi-season pack with one
		// subfolder per season, flattened by collectVideoFiles) would point at the same dest, and
		// the second would fall into the "different bytes" branch removing the first's link — an
		// episode would vanish from the library. The relative path is unique inside the torrent.
		if used[destName] {
			destName = strings.ReplaceAll(rel, string(filepath.Separator), " - ")
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

	// After the links: if it failed earlier, cleanupIfEmpty would not be able to remove the folder.
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

// writeShowNFO writes tvshow.nfo with the AniList id so Jellyfin (AniList plugin)
// matches by id. Does not overwrite an existing nfo — the user may have adjusted the match
// by hand. Failure here does not invalidate the hardlinks, so it only logs. Returns true if it wrote.
func (o *organizer) writeShowNFO(destDir, animeName string, animeID int) bool {
	if animeID <= 0 {
		return false
	}
	path := filepath.Join(destDir, "tvshow.nfo")
	if _, err := o.fs.Stat(path); err == nil {
		return false
	}

	data, err := xml.MarshalIndent(nfoTVShow{
		Title: animeName,
		// "AniList" with this capitalization is the value of ProviderNames.AniList in
		// jellyfin-plugin-anilist. Jellyfin's ProviderIds is OrdinalIgnoreCase, so
		// lowercase would also match — we write it equal to the provider so we don't depend on that.
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

// BackfillShowNFOs writes tvshow.nfo for folders that were already in the library before the
// nfo existed: Organize only runs for a new episode (it returns early when LibraryPaths is
// already filled), so without this an anime that already finished would never get the file.
// The folder comes from LibraryPaths, not from the anime name, so it matches what was actually
// created on disk (sanitization, manual rename). One folder per anime; a folder gone from
// disk is skipped.
//
// MUST ONLY RUN WITH AnimeIDs ALREADY MIGRATED to media ids (decisions.md #43): because the
// nfo is never overwritten, writing an entry id here would be permanent.
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
			continue // folder removed from the library by something else
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

	// The leading dot hides the folder from Jellyfin's scanner on Linux; .ignore covers
	// platforms where a leading dot does not mark hidden. Both defenses together, because the
	// download folder now lives inside the folder Jellyfin scans.
	ignorePath := filepath.Join(downloadPath, ".ignore")
	if _, err := o.fs.Stat(ignorePath); err != nil {
		if err := o.fs.WriteFile(ignorePath, nil, 0644); err != nil {
			return fmt.Errorf("cannot write ignore marker %s: %w", ignorePath, err)
		}
	}

	probeSrc := filepath.Join(downloadPath, ".aad_link_probe")
	probeDst := filepath.Join(completedPath, ".aad_link_probe")

	// Clean leftovers from a previous probe.
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
