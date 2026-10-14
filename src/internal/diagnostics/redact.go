package diagnostics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

type keyClass int

const (
	keep         keyClass = iota // value kept; strings still go through scrub
	drop                         // key removed from the line
	pseudonymize                 // account name: becomes user1, user2, ...
	title                        // reveals what the user downloads: <title-N> when titles are off
)

// keyClasses classifies every log field key the code writes. TestEveryLogKeyIsClassified fails when
// a key in the source isn't here, so a new field can't leak by default (decisions.md #103). A key
// missing from the table at runtime (a field removed from the code, still in an old log) is treated
// as a title: hidden when titles are off.
var keyClasses = map[string]keyClass{
	// zerolog's own fields
	"level": keep, "time": keep, "caller": keep, "message": keep, "error": keep, "stack": keep,

	"remote_addr": drop,

	"username": pseudonymize, "usernames": pseudonymize, "mal_user": pseudonymize, "mal_usernames": pseudonymize,

	"anime": title, "anime_name": title, "title": title, "term": title, "name": title,
	"torrent": title, "torrents": title, "matched_torrents": title, "hash": title,
	// Nyaa URLs: their query is the search term.
	"url": title,
	// Library paths end in the anime's folder and file names.
	"path": title, "folder_path": title, "source": title, "destination": title,
	"anilist_response": title,
	// An AniList/MAL id is a title by lookup.
	"anime_id": title, "media_id": title, "mal_id": title, "owner_anime_id": title,
	"entries_no_longer_on_anilist": title,

	"absolute_episode": keep, "addr": keep, "animes_checked": keep, "animes_found": keep,
	"attempt": keep, "attempts": keep, "avg_time_per_episode": keep, "body": keep, "body_size": keep,
	"clients": keep, "completed_anime_path": keep, "count": keep, "debug-anime": keep,
	"debug_dir": keep, "download_path": keep, "downloaded_episodes": keep, "duration": keep,
	"earlier_episode_missing": keep, "end_episode": keep, "entries_resolved": keep,
	"environment": keep, "episode": keep, "episode_number": keep, "episodes": keep,
	"episodes_checked": keep, "episodes_count": keep, "episodes_downloaded": keep,
	"episodes_rewritten": keep, "episodes_would_search": keep, "files": keep,
	"first_episode": keep, "frontend_lists": keep, "graphql_message": keep, "graphql_status": keep, "id": keep,
	"last_episode": keep, "lines": keep, "magnets_found": keep,
	"max_attempts": keep, "max_bytes": keep, "max_episodes": keep, "media_entries": keep,
	"media_ids": keep, "media_status": keep, "method": keep, "min_seeders": keep, "new": keep,
	"old": keep, "orphans": keep, "packs": keep, "pass_lists": keep, "pieces_have": keep,
	"pieces_total": keep, "port": keep, "progress": keep, "resolved": keep, "results": keep,
	"retry_in": keep, "save_path": keep, "saved_at": keep, "season": keep, "seed": keep,
	"seeders": keep, "series_links": keep, "size_bytes": keep, "standalone_orphans": keep,
	"start_episode": keep, "status": keep, "status_code": keep, "stalled_for": keep,
	"time_until_airing": keep, "torrents_found": keep, "total_attempts": keep,
	"total_time": keep, "type": keep, "webhook": keep,
}

// The hosts the app itself talks to. Any other URL in the text is replaced: a webhook that was
// rotated or removed still sits in old log lines, and the current config no longer knows it.
var knownHostSuffixes = []string{"nyaa.si", "anilist.co", "myanimelist.net", "animetosho.org", "localhost", "127.0.0.1"}

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// redactor holds what one bundle learned from the config, and the pseudonyms it handed out, so the
// same user or title gets the same token across the whole bundle.
type redactor struct {
	titles bool
	home   string

	webhookSecrets []string // literal → <webhook-N>, longest first
	users          map[string]string
	userPatterns   []userPattern
	titleTokens    map[string]string
}

type userPattern struct {
	re    *regexp.Regexp
	alias string
}

func newRedactor(titles bool, home string, usernames []string, webhookSecrets []string) *redactor {
	r := &redactor{titles: titles, users: map[string]string{}, titleTokens: map[string]string{}}
	// A home of "/" would turn every path into "~...".
	if len(home) > 1 {
		r.home = strings.TrimRight(home, `/\`)
	}
	for _, s := range webhookSecrets {
		if s = strings.TrimSpace(s); len(s) >= 8 {
			r.webhookSecrets = append(r.webhookSecrets, s)
		}
	}
	for _, name := range usernames {
		r.alias(name)
	}
	return r
}

// alias returns the user's pseudonym, assigning the next one on first sight. Case-insensitive:
// AniList and MAL usernames are.
func (r *redactor) alias(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	key := strings.ToLower(name)
	if a, ok := r.users[key]; ok {
		return a
	}
	a := fmt.Sprintf("user%d", len(r.users)+1)
	r.users[key] = a
	r.userPatterns = append(r.userPatterns, userPattern{re: wordPattern(name), alias: a})
	return a
}

// wordPattern matches the name only as a whole word, so a username like "test" doesn't mangle
// "latest". Quoting a string known from the config is not free-text guessing.
func wordPattern(name string) *regexp.Regexp {
	expr := regexp.QuoteMeta(name)
	if isWordRune(rune(name[0])) {
		expr = `\b` + expr
	}
	if isWordRune(rune(name[len(name)-1])) {
		expr += `\b`
	}
	return regexp.MustCompile(`(?i)` + expr)
}

func isWordRune(c rune) bool { return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) }

func (r *redactor) titleToken(value string) string {
	if t, ok := r.titleTokens[value]; ok {
		return t
	}
	t := fmt.Sprintf("<title-%d>", len(r.titleTokens)+1)
	r.titleTokens[value] = t
	return t
}

// scrub applies the text-level rules to any string: webhook secrets, home, unknown URLs, usernames.
func (r *redactor) scrub(s string) string {
	for i, secret := range r.webhookSecrets {
		s = strings.ReplaceAll(s, secret, fmt.Sprintf("<webhook-%d>", i+1))
	}
	// Home before usernames: the login name is often an account name too, and replacing it first
	// would leave "/home/user1" for the home rule to miss.
	if r.home != "" {
		s = strings.ReplaceAll(s, r.home, "~")
	}
	s = urlPattern.ReplaceAllStringFunc(s, r.scrubURL)
	for _, u := range r.userPatterns {
		s = u.re.ReplaceAllLiteralString(s, u.alias)
	}
	return s
}

func (r *redactor) scrubURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<url>"
	}
	host := strings.ToLower(parsed.Hostname())
	for _, known := range knownHostSuffixes {
		if host == known || strings.HasSuffix(host, "."+known) {
			if known == "nyaa.si" && !r.titles {
				return r.titleToken(raw)
			}
			return raw
		}
	}
	return "<url>"
}

// value redacts one decoded JSON value according to its key's class.
func (r *redactor) value(class keyClass, v any) any {
	switch class {
	case pseudonymize:
		return mapStrings(v, r.alias)
	case title:
		if !r.titles {
			return r.hideTitle(v)
		}
	}
	return mapStrings(v, r.scrub)
}

func (r *redactor) hideTitle(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = r.hideTitle(item)
		}
		return out
	case string:
		return r.titleToken(t)
	case nil, bool:
		return t
	default: // numbers (ids) and objects (an AniList response)
		return r.titleToken(encodeJSON(t))
	}
}

func mapStrings(v any, f func(string) string) any {
	switch t := v.(type) {
	case string:
		return f(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = mapStrings(item, f)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = mapStrings(item, f)
		}
		return out
	}
	return v
}

// logLine redacts one log line. ok is false when the line is dropped. The line is walked token by
// token, not decoded into a map: that keeps the field order, and a line can carry the same key twice
// (an explicit "message" field next to zerolog's own) — a map would silently keep only one.
func (r *redactor) logLine(line string) (out string, ok bool) {
	if isHTTPRequestLine(line) {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return r.scrub(line), true
	}

	var b strings.Builder
	b.WriteByte('{')
	first := true
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return r.scrub(line), true
		}
		key, _ := keyTok.(string)
		var v any
		if err := dec.Decode(&v); err != nil {
			return r.scrub(line), true
		}
		class, known := keyClasses[key]
		if !known {
			class = title
		}
		if class == drop {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(encodeJSON(key))
		b.WriteByte(':')
		b.WriteString(encodeJSON(r.value(class, v)))
	}
	b.WriteByte('}')
	return b.String(), true
}

// isHTTPRequestLine spots middleware.go's per-request lines: the majority of the log, carrying LAN
// IPs and explaining nothing.
func isHTTPRequestLine(line string) bool {
	return strings.Contains(line, `"message":"HTTP request"`)
}

// encodeJSON never escapes <, > and &: the <title-N> tokens have to stay readable.
func encodeJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return `"<unencodable>"`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
