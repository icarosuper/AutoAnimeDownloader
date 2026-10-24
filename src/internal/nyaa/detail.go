package nyaa

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ErrTorrentNotFound is returned when Nyaa answers 404 for a view page.
var ErrTorrentNotFound = errors.New("torrent not found")

// ErrNyaaUnavailable is returned when Nyaa is unreachable or answers a non-404 failure.
var ErrNyaaUnavailable = errors.New("nyaa unavailable")

// TorrentFile is one file on a Nyaa view page (folders are never rows).
type TorrentFile struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// TorrentDetail is the sanitised description and flattened file list of a Nyaa view page.
type TorrentDetail struct {
	DescriptionHTML string        `json:"description_html"`
	Files           []TorrentFile `json:"files"`
	NyaaURL         string        `json:"nyaa_url"`
}

// ParseNyaaID returns the numeric id from a /view/<id> URL. Non-digits or missing → 0.
func ParseNyaaID(detailURL string) int {
	const marker = "/view/"
	i := strings.LastIndex(detailURL, marker)
	if i < 0 {
		return 0
	}
	rest := detailURL[i+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	id, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return id
}

// ViewURL is the absolute /view/<id> page for a Nyaa id, including the NYAA_URL override.
func ViewURL(nyaaID int) string {
	return getNyaaBaseURL() + "/view/" + strconv.Itoa(nyaaID)
}

// FetchTorrentDetail loads a Nyaa view page by id. The URL is built here on purpose — a caller
// never supplies one (decisions.md #105).
func FetchTorrentDetail(nyaaID int) (TorrentDetail, error) {
	if nyaaID <= 0 {
		return TorrentDetail{}, fmt.Errorf("invalid nyaa id %d", nyaaID)
	}

	viewURL := ViewURL(nyaaID)
	doc, err := fetchNyaaPage(viewURL)
	if err != nil {
		var httpErr *nyaaHTTPError
		if errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound {
			return TorrentDetail{}, fmt.Errorf("torrent %d not found: %w", nyaaID, ErrTorrentNotFound)
		}
		return TorrentDetail{}, fmt.Errorf("failed to fetch nyaa view %d: %w", nyaaID, ErrNyaaUnavailable)
	}

	desc, err := doc.Find("#torrent-description").Html()
	if err != nil {
		desc = ""
	}

	return TorrentDetail{
		DescriptionHTML: sanitizeDescriptionHTML(desc),
		Files:           flattenTorrentFiles(doc),
		NyaaURL:         viewURL,
	}, nil
}

// nyaaHTTPError carries a non-200 status from fetchNyaaPage so FetchTorrentDetail can tell 404
// from other failures. PackFileRange only checks err != nil.
type nyaaHTTPError struct {
	Status int
}

func (e *nyaaHTTPError) Error() string {
	return fmt.Sprintf("HTTP error: status %d", e.Status)
}

var descriptionAllowedTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "pre": true, "code": true, "blockquote": true,
	"br": true, "em": true, "strong": true, "b": true, "i": true, "a": true, "img": true,
}

// sanitizeDescriptionHTML keeps a small allow-list of formatting tags, strips scripts/iframes/
// event handlers, and resolves relative img src against the Nyaa base URL.
func sanitizeDescriptionHTML(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<div id="__root">` + raw + `</div>`))
	if err != nil {
		return ""
	}
	root := doc.Find("#__root")
	root.Find("script,iframe,style").Remove()

	base := getNyaaBaseURL()
	root.Find("*").Each(func(_ int, s *goquery.Selection) {
		cleanDescriptionAttrs(s, base)
	})

	// Unwrap unknown tags deepest-first so children stay in place.
	for {
		var unknown *goquery.Selection
		root.Find("*").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if descriptionAllowedTags[goquery.NodeName(s)] {
				return true
			}
			unknown = s
			return false
		})
		if unknown == nil {
			break
		}
		if unknown.Contents().Length() == 0 {
			unknown.Remove()
			continue
		}
		unknown.Contents().Unwrap()
	}

	out, err := root.Html()
	if err != nil {
		return ""
	}
	return out
}

func cleanDescriptionAttrs(s *goquery.Selection, base string) {
	node := s.Get(0)
	if node == nil {
		return
	}
	tag := goquery.NodeName(s)
	// Copy first: ranging Attr while appending into Attr[:0] aliases the same array.
	attrs := append(node.Attr[:0:0], node.Attr...)
	kept := attrs[:0]
	for _, attr := range attrs {
		key := strings.ToLower(attr.Key)
		if strings.HasPrefix(key, "on") {
			continue
		}
		switch tag {
		case "a":
			if key != "href" || !safeHref(attr.Val) {
				continue
			}
		case "img":
			if key != "src" {
				continue
			}
			src, ok := resolveImgSrc(attr.Val, base)
			if !ok {
				continue
			}
			attr.Val = src
		default:
			// Allowed tags keep no attributes other than a.href / img.src.
			continue
		}
		kept = append(kept, attr)
	}
	node.Attr = kept
}

func safeHref(v string) bool {
	u, err := url.Parse(v)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	default:
		return false
	}
}

func resolveImgSrc(v, base string) (string, bool) {
	u, err := url.Parse(v)
	if err != nil {
		return "", false
	}
	if u.Scheme == "" {
		if !strings.HasPrefix(v, "/") {
			return "", false
		}
		return base + v, true
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return v, true
	default:
		return "", false
	}
}

// flattenTorrentFiles walks the view-page file list the way walkFileList does for folders, but
// emits every file (video and other). Nested names keep parent/child.
func flattenTorrentFiles(doc *goquery.Document) []TorrentFile {
	var files []TorrentFile
	walkTorrentFiles(doc.Find(".torrent-file-list > ul"), "", &files)
	return files
}

func walkTorrentFiles(sel *goquery.Selection, path string, out *[]TorrentFile) {
	sel.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
		if link := li.Children().First(); link.Is("a.folder") {
			child := strings.TrimSpace(link.Text())
			sub := child
			if path != "" {
				sub = path + "/" + child
			}
			walkTorrentFiles(li.ChildrenFiltered("ul"), sub, out)
			return
		}
		name := fileListEntryName(li)
		if name == "" {
			return
		}
		full := name
		if path != "" {
			full = path + "/" + name
		}
		size := parseSize(strings.Trim(strings.TrimSpace(li.Find("span.file-size").Text()), "()"))
		*out = append(*out, TorrentFile{Name: full, SizeBytes: size})
	})
}
