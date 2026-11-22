# 50. Standalone search: the unreleased filter is server-side and by concatenation, and a blocked card becomes a link instead of a dead end

**Location:** `anilist/standalone.go` — `SearchMedia`; `api/endpoint_anilist_search.go`; `frontend/src/routes/AddAnime.svelte`; `frontend/src/components/ui/Button.svelte`.

**What it looks like:** a `status_not: NOT_YET_RELEASED` **stuck onto the query string** instead of passed as a GraphQL variable; a toggle that does not know how many results it hid; and a `Button` that sometimes renders `<a>`.

**Why it's right:**

**(a) The filter lives in the query, not in Go.** `perPage` is 20. Filtering after receiving would return searches with 4 useful results in announcement season — the user would read that as "search is broken", not as "I hid 16". The cost is that **nothing knows how many were hidden**, so the toggle is blind: there is no "3 hidden · show". That was the chosen trade; inverting the two is a product decision, not a bug.

**(b) Concatenation, not a null GraphQL variable.** AniList ignores arguments with a null value today, and `status_not: $statusNot` with `$statusNot = null` would work. But if that semantics changes, the filter starts hiding everything or nothing **in silence** — the worst failure mode for a filter. With concatenation, `includeUnreleased = true` makes the argument simply not exist in the query, which is true on any API version. It costs three lines and the test inspects the body sent, not the result, precisely because both implementations would pass a result test.

**(c) Only `NOT_YET_RELEASED` leaves.** `CANCELLED` and `HIATUS` stay: an anime interrupted mid-airing has episodes out and downloadable. "Not released yet" is one status.

**(d) `tracked`/`downloaded`/`standalone` become a link to `#/status/{id}`.** `anime_id` **is** AniList's media id, so the href comes from the search result itself, with no lookup — and `AddStandaloneAnime` writes before answering, so the link works the instant after the Add click, without waiting for a daemon cycle. The block reason left the `tooltip` and became a line on the card: tooltips do not exist on mobile, and the screen was born for the no-account install, which is the one that runs on a phone the most. Every block reason has a page now; the greyed, disabled `blacklist` card went away with the reason ([#49](049-standalone-anime-tracked-by-the-list-is-the.md) (c)).

**(e) A `Button` with `href` renders `<a>` instead of an `on:click` that writes `location.hash`.** A button that navigates loses middle-click, "open in new tab" and the URL in the status bar. The alternative was copying the variant classes into the card, which would duplicate the style on every screen that needs it. `disabled` is ignored with `href` on purpose: there is no disabled anchor, and a link that must not be followed must not be rendered as a link.

**(f) Search fires on submit (button or Enter), never on every key.** The original version had a 300ms debounce. The real cost is not the fast typist — they already spent 1 request per search — it is whoever types with more than 300ms between keys (phone, hunt-and-peck): from the 3rd character they spent **one request per key**. Because `SearchMedia` is `PriorityDisposable`, whoever blew the bucket was refused by `budgetAllows` on the *next* search — the user saw "search failed" for having typed slowly. With submit, one search = one request, and the budget floor is never hit by the screen. The `<form>` exists so Enter is native behaviour instead of an `on:keydown`; the `AbortController` stays because two submits in a row still race each other. The unreleased toggle only redoes the search when a result is already on screen — with no search done, it only picks the parameter of the next submit.

**(g) The search cache lives in `SearchMedia`, not on the endpoint response.** `block_reason` is recomputed by `standaloneGuard` on every response; cached with it, an anime added just now would keep the "Add" button on the card until the TTL expired. Caching only the part that comes from AniList keeps the guard fresh for free. The key is `strings.ToLower(term)+includeUnreleased` — AniList search is case-insensitive, so "One Piece" and "one piece" are not worth two requests. It started paying off together with (f): with submit the keys are whole terms, which repeat (the same user redoing the search, the toggle going back and forth, another tab); with debounce they were prefixes nobody repeated. It is also **the only cache with a free-text key** — the others are anime id and account name, a finite set — and because `ttlCache` never removes an expired entry (`getStale` depends on that), it is the only one that needs a ceiling: past `searchCacheMaxEntries` the map is discarded whole. Whole discard and not LRU on purpose; the `ponytail:` in the code notes the upgrade.

**Don't "fix" by:**
- Moving the filter into Go "so we can show how many were hidden": that reintroduces the search with 4 of 20 (see (a)).
- Going back to searching on `on:input`, with or without debounce "because live results are better UX": AniList's budget is 30 req/min per IP and the search is disposable (see (f)).
- Lifting the search cache onto the endpoint response "because that's one fewer layer": `block_reason` freezes with it (see (g)).
- Caching the search in the frontend instead of the backend: AniList's limit is per IP, so a per-tab cache does not protect the other tabs or the daemon pass — and the budget gate, which is who the cache exists to spare, lives in the backend (see (g)).
- Swapping concatenation for `status_not: $var` with a null value "because a variable is cleaner" (see (b)).
- Extending the filter to `CANCELLED`/`HIATUS` — those have downloadable episodes (see (c)).
- Putting `block_reason` back inside a `tooltip` "to keep the card cleaner": the reason vanishes on all of mobile.
- Making the whole card an `<a>`: the Add button inside the anchor would need `stopPropagation` and would break keyboard navigation.
