package daemon

import "errors"

// Causes of an aborted pass. They are CODES the frontend translates — the same boundary
// lib/domain/checkIssue.ts already holds: the backend sends a code, the frontend builds the
// sentence. Before this the Status screen banner showed the raw `error.Error()`, which in
// practice meant dumping an AniList response JSON in the user's face.
const (
	PassErrConfig  = "config"          // couldn't read or write the configuration
	PassErrSetup   = "setup"           // setup is incomplete (library folder)
	PassErrLibrary = "library"         // the library folder doesn't accept hardlinks
	PassErrTorrent = "torrent_backend" // the torrent client didn't start
	PassErrAnilist = "anilist"         // AniList didn't answer
	PassErrMAL     = "mal"             // every MyAnimeList account failed and nothing else answered
	PassErrStorage = "storage"         // the app's own data (episodes.json, migrations)
	PassErrUnknown = "unknown"
)

// Sentinels wrapped into the error at the abort site. The code travels INSIDE the error instead
// of in a new SetLastCheckError parameter: there are ~15 calls today, most of them with nil to
// clear, and none of them needs to change. A new site that forgets to wrap falls into
// PassErrUnknown, which has its own sentence — it degrades, it doesn't break.
var (
	errCauseConfig  = errors.New("configuration")
	errCauseSetup   = errors.New("incomplete configuration")
	errCauseLibrary = errors.New("library")
	errCauseTorrent = errors.New("torrent client")
	errCauseAnilist = errors.New("anilist")
	errCauseMAL     = errors.New("myanimelist")
	errCauseStorage = errors.New("app data")
)

// passErrorCode translates an aborted pass's error into the code the frontend understands.
func passErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errCauseConfig):
		return PassErrConfig
	case errors.Is(err, errCauseSetup):
		return PassErrSetup
	case errors.Is(err, errCauseLibrary):
		return PassErrLibrary
	case errors.Is(err, errCauseTorrent):
		return PassErrTorrent
	// Before AniList: searchAnilist's error is wrapped in errCauseAnilist at the abort site, so a
	// MAL-only failure carries both sentinels and the more specific one has to win.
	case errors.Is(err, errCauseMAL):
		return PassErrMAL
	case errors.Is(err, errCauseAnilist):
		return PassErrAnilist
	case errors.Is(err, errCauseStorage):
		return PassErrStorage
	default:
		return PassErrUnknown
	}
}
