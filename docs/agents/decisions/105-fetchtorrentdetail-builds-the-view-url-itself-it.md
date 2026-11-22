# 105. FetchTorrentDetail builds the view URL itself; it never takes one from the caller

**Location:** `nyaa/detail.go` (`FetchTorrentDetail`, `ParseNyaaID`); consumed later by the
standalone torrent picker's description pane.

**The rule:** the only input is a numeric Nyaa id. The view URL is always
`getNyaaBaseURL()+"/view/"+strconv.Itoa(nyaaID)`. A caller that already has a `DetailURL` parses
the id with `ParseNyaaID` and hands that over — it does not pass the string through.

**Why:** the API that will expose this will take an id from the client. Accepting a URL would let
the client point the daemon at any host (`NYAA_URL` is the operator's choice; a request body is
not). Building the URL here keeps SSRF out of every future caller by construction.

**Don't "fix" by:** adding a `FetchTorrentDetailURL(url string)` convenience that "just" reuses
the same parser, or by letting the API accept a URL "because the UI already has it".
