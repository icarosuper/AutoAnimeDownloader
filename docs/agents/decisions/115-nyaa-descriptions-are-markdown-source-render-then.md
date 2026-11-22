# 115. Nyaa descriptions are markdown source; render then sanitise

**Location:** `internal/nyaa/detail.go` — `descriptionInnerHTML` / `sanitizeDescriptionHTML`.

**What it looks like:** goldmark runs on the view-page description, then the HTML
allow-list strips tags again. `{@html}` in the picker is still fed sanitised HTML.

**Why:** live Nyaa does not send HTML. `#torrent-description` is escaped markdown
with newlines as `&#10;` (`markdown-text`); the browser's markdown-it renders it.
Dumping that source into `{@html}` collapses whitespace and shows `**bold**` and
pipe-tables as one blob. goldmark (GFM, `WithUnsafe`) is the same shape as Nyaa's
`html: true` markdown-it. We are not Nyaa, so the sanitiser stays: `script` /
`iframe` / `style` / `on*` / non-http(s)/mailto hrefs go. `img` keeps http(s) `src`
and `alt`, always sets `referrerpolicy=no-referrer` (hosts like extraimage.info
hotlink-check `Referer`), and drops a tag that lost its `src` so `javascript:`
does not become a broken icon. A fixture that already has element children skips
the markdown pass (the existing sanitiser test).

**Don't "fix" by:** injecting the view page unsanitised; iframe-ing Nyaa; skipping
goldmark and using `white-space: pre-wrap` on the source (tables and images stay
unreadable).
