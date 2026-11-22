# 66. A banner is degraded state; a toast is an action failure — and the banner has three sources with precedence, not three banners

**Location:** policy for `components/shell/AppShell.svelte` (single banner slot) and `lib/api/client.ts` (`ApiError`, `silent` option).

**What it looks like:** each error origin wants its own notice, and the obvious way out is to stack — an AniList banner, a backend one, the WebSocket one that already exists in `routes/Downloads.svelte`.

**Why it's right:** the distinction is not severity, it is **duration**:

- **Toast** — an action the user asked for failed (save config, add anime). Already exists, already works, goes away on its own.
- **Banner** — state is degraded between polls and explains why the whole screen looks wrong. Persists while the state persists and **goes away on the first success**, never by timer.

The banner's value is not reporting the error: it is explaining why the list is incomplete or stale. Today `/animes` answers 200 with an account missing and the user concludes an anime vanished.

The sources are independent, but **stacking is wrong**: if the backend does not answer, AniList health is stale information from the last successful poll, and showing it next to "daemon does not respond" is contradictory noise. Precedence, from furthest upstream to furthest downstream (`lib/domain/systemBanner.ts`, `pickBanner`):

1. **Backend unreachable** (`fetch` got no response) — nothing below that is observable.
2. **Backend answering 5xx** — our bug.
3. **AniList unavailable** (403/5xx) — mute banner, "try later".
4. **AniList rate limited** (429) — countdown derived from `Retry-After`, the only case where the exact time is known.

**One** is shown, the highest precedence.

**The WebSocket banner of `routes/Downloads.svelte` stays where it is.** It talks about that screen — progress there is pushed over WS and goes stale without it — while `SystemBanner` talks about the whole app. Consolidating them would require moving ownership of `WebSocketClient` to `AppShell` (today instantiated in `Status` and `Downloads`), which is a refactor of another subject. Absence of the daemon is already covered by the `unreachable` state, which comes from HTTP.

**The report button only appears on our bug** — backend 5xx, and AniList 400 (schema changed, no wait resolves it). A 429 or an outage is not a bug: a report button there only generates an issue that closes with "it's AniList", and trains the user to ignore the button when it matters.

**Blind spot this closes:** `getLastCheck` and `getTorrents` pass `silent: true` so they do not toast every tick. Correct for toast — but today it means a repeated 5xx on those endpoints is **invisible forever**. Backend health is derived from *consecutive* failures per endpoint (≥2), not from an isolated failure, and that is why it coexists with `silent` without reintroducing the spam.

**`pass_error` follows the same rule.** The aborted-pass banner on the Status screen showed raw `err.Error()` — which, with AniList down, was the entire response JSON body (`{"errors":[{"message":…,"locations":[{"line":1,"column":1}]}]}`) in the user's face. `CheckReport` gained `pass_error_code`, classified in `daemon/passerror.go`, and the phrase is built in `lib/domain/passError.ts`. The raw text stays in `pass_error` and is shown **collapsed** in a `<details>`: it is the only thing pasteable into an issue, but it cannot be the first thing you read.

The cause travels **inside** the error (sentinels wrapped with `%w`), not in a new `SetLastCheckError` parameter: there are ~15 call sites, most passing `nil` to clear, and none of them needed to change. A new abort site that forgets to wrap falls into `unknown`, which has its own phrase — degrades, does not break.

**Don't "fix" by:**
- Stacking the banners "because they are different problems" — with the backend down, AniList state on screen is a dated lie.
- Making the banner vanish by timer — the state either ended (a 200 arrived) or it did not.
- Putting a report button on every banner "for symmetry" — reporting a rate limit is reporting normal behaviour.
- Swapping the outage banner for a toast "because it's an error" — a toast goes away, the outage does not; the screen stays wrong after it vanished.
- Sending a ready-made phrase from Go instead of a code, "because it's just one string" — the daemon does not know the browser's locale, and that is how the raw JSON reached the screen.
- Removing the `<details>` of the raw text "because the user doesn't understand it" — whoever opens an issue needs it, and collapsed it does not get in the way of whoever does not.
