# Bug report

**Date:** 2026-10-13
**Status:** approved design, not implemented
**Origin:** `v2.5.0` in [TODO.md](../../TODO.md) — "Bug report mechanism — must exist well before
promoting the project, otherwise feedback arrives without context"
**Expected scope:** new `src/internal/diagnostics` and `src/internal/datadir` packages, `src/internal/api` (one endpoint, one
status field), `src/cmd/daemon/main.go` (crash capture), `src/internal/tray` (one menu item),
frontend (new `#/report` route, toast store, i18n en + pt-BR), `.github/ISSUE_TEMPLATE/`, docs.

---

## The idea in one sentence

The daemon builds a **redacted diagnostics bundle locally**, the report screen shows the user
exactly what is in it, and the user attaches it to a GitHub issue by their own hand. **Nothing leaves the machine without the user acting.**

## Why there is no backend

The project has no server: only the GitHub repo, and the landing page doesn't exist yet. An
anonymous form would need a Worker holding a GitHub token — a spam vector and one more thing to
keep running. Requiring a GitHub account is a barrier for lay users, accepted for now; the Worker
gets built only if reports don't arrive.

The consequence that shapes the rest: consent is the act of attaching. There is no "send" button
that ships data anywhere, so there is no opt-in to design — only a **preview** to make honest.

## Entry points

| Entry | Opens | Pre-filled with |
|---|---|---|
| "Report a problem" item in the `MoreMenu` (`navItems.ts`, next to Logs) | `#/report` | nothing |
| "Report a problem" item in the tray menu (`tray.go`, between "Check for new episodes" and "Quit") | browser at `#/report` | nothing |
| "Report" link on **unexpected** error toasts — the automatic ones from `client.ts` with status 5xx, network failure (status 0) or unknown error | `#/report?error=…&context=…&anime=…` | the error message; `context` = `METHOD endpoint status`; `anime` = the media ID when the failing endpoint is `/animes/{id}/…` |
| Pending crash | `#/report?crash=1` | the crash output — see "Crash capture" for when it opens by itself |

The report screen is a **route**, not a modal: the tray and the toast link both need a URL to land
on, and a route gets that for free from `svelte-spa-router` (`$querystring` is already used by
`Downloads.svelte`).

4xx toasts (validation, "username not found") get **no** link: they're the app answering
correctly, and a link there would turn them into junk issues. Page-level `toast.error` calls don't
get one either unless the page opts in.

Error toasts carrying the link stay up for 10 s instead of the default 4 s — 4 s isn't enough to
read a message and decide to click.

The error toast entry depends on fix 1 in "Fixes bundled with this work" below: today the
automatic error toasts don't show up at all.

## The report screen (`#/report`)

Top to bottom:

1. **"What happened?"** — free-text field, required. The only field the user must fill.
2. **Pre-filled context**, read-only, shown only when present: the error from the toast, or
   "AutoAnimeDownloader stopped unexpectedly on <date>" for a crash.
3. **"Include anime titles and torrent names"** — checkbox, **checked by default**. One line under
   it saying why it's there: titles are needed to debug almost any search problem, and they also
   show what you download.
4. **Preview** — the full bundle, read-only, scrollable, regenerated when the checkbox changes.
5. A notice: **attachments on GitHub issues are public and permanent.**
6. Two actions:
   - **Open GitHub issue** — downloads the bundle as
     `aad-report-<version>-<yyyymmdd-hhmm>.txt`, then opens the issue form in a new tab with the
     title, the "what happened" text and the environment summary pre-filled. The screen then says:
     "drag the downloaded file into the issue". GitHub has no way to pre-attach a file, and the log
     can't travel in the URL (URLs get cut off at a few KB).
   - **Download report** — saves the same `.txt` and does nothing else: no tab opens, nothing is
     sent. For the user who wants to send it some other way (no GitHub account, a message to the
     maintainer, a forum, an AI chat) or just read it first. It also counts as "handled" for a
     pending crash (see "Crash capture").

   "Open GitHub issue" downloads the file and calls `window.open` **synchronously in the click
   handler**, from the bundle already loaded for the preview — an `await` in between and popup
   blockers swallow the tab.

The repo URL lives in **one** frontend constant: the `v3.0.0` rename to Serval changes it.

## The bundle

A single plain-text file (not a zip): readable in the preview and in the GitHub attachment
viewer. Sections, in order:

1. **Environment**
   - app version (`version.Version`), OS and arch (`runtime.GOOS`/`GOARCH`)
   - Linux: distro (`PRETTY_NAME` from `/etc/os-release`), desktop environment
     (`XDG_CURRENT_DESKTOP`), session type (`XDG_SESSION_TYPE`, Wayland vs X11). The desktop
     matters because tray behavior differs per desktop (GNOME has no native tray)
   - Windows: the version reported by the OS
   - **running in docker** (`/.dockerenv` exists) — then the distro is the container's, so it's
     reported as `docker` and the distro line is omitted instead of lying
   - **running under systemd** (`INVOCATION_ID` is set) — then the desktop variables don't reach
     the process, so they're reported as `unknown`, not `none`
   - free disk on the library volume and whether it's below `min_free_disk_percent` (the same
     numbers `/api/v1/status` already computes, through `files.DiskSpace` — `diagnostics` imports
     `files`, not `api`, so there's no cycle): a full disk silently stops
     new downloads
2. **Context** — the toast error / `context` / anime media ID when present. The media ID is what
   lets the maintainer run `make debug-anime ID=…` and reproduce the search
   ([troubleshooting-downloads.md](../../agents/troubleshooting-downloads.md), Step 0).
3. **Crash** — the pending crash output, when there is one.
4. **Config** — `config.json`, redacted (see below), plus `anime_settings.json` (keyed by media
   ID, no personal data).
5. **Log** — the last 1000 lines, redacted. When the active `daemon.log` has fewer than that (right
   after a rotation), the rest comes from the most recent **`.gz` backup** (`lumberjack` runs with
   `Compress: true`; `/api/v1/logs` reads only the active file and doesn't handle this). Lines stay
   JSON — that's what the maintainer greps.

**HTTP request lines (`"message":"HTTP request"`, from `middleware.go`) are dropped.** In the real
log they're the majority of lines, they carry LAN IPs, and they explain nothing.

The install method (AUR, docker, Windows installer, plain binary) is **not** detected: the binary
can't tell AUR from a hand-extracted release. It's a dropdown in the issue form instead — the user
knows the answer.

No CPU, RAM or machine model: none of them explains a bug in this app. Add one when a performance
bug needs it.

## Privacy

### What leaks today

Concrete, from the real `daemon.log` and the code:

- `remote_addr` — LAN IPs, on every HTTP request line
- `usernames` / `username` / `mal_user` — AniList and MAL accounts
- the home directory inside `download_path` and other paths (`/home/<user>/…` reveals the login
  name)
- **webhook URLs inside `error`**: `notifications.go:192` logs `.Err(err)` of a failed webhook
  request, and Go's HTTP client puts the full URL in the error string
  (`Post "https://discord.com/api/webhooks/<id>/<token>": …`). A Discord webhook URL **is** the
  secret
- in the config: webhook `url`, `headers` and `body` (tokens, auth headers)

### Redaction rules

| Level | What | How |
|---|---|---|
| **Never included** | `remote_addr`, hostname | key dropped |
| **Never included** | webhook `url`, `headers`, `body` in the config | replaced by `<redacted>`; name and method stay (the name is a user-chosen label and is what the log's `webhook` key refers to) |
| **Never included** | the configured webhook URLs and header values, **anywhere in the text** | literal replacement by `<webhook-N>`. A URL with `{{vars}}` is matched by its static prefix up to the first `{{` |
| **Never included** | any URL whose host isn't one the app talks to (nyaa, anilist, myanimelist, animetosho, localhost), inside `error`, `body` and other free-text values | replaced by `<url>`. This covers what the literal replacement can't: a webhook that was **rotated or removed** still sits in older log lines, and the current config doesn't know it anymore |
| **Pseudonymized** | AniList / MAL usernames | `user1`, `user2`, … — stable within one bundle, applied to keys **and** as a literal replacement in any string (they also appear inside messages and errors). The count is kept because multi-account bugs depend on it |
| **Pseudonymized** | the home directory | replaced by `~` in every string |
| **Optional, on by default** | anime titles, episode names, torrent names, hashes, search terms, Nyaa URLs (their query **is** the search term), standalone anime IDs | when off: values replaced by `<title-N>`, stable within the bundle so the maintainer can still follow one anime through the log |

Redaction is **by key** on parsed JSON, plus literal replacement of the strings known from the
config (usernames, webhook URLs, home), plus the URL host allowlist above. No other regexes over
free text. A username removed from the config before the bundle is built isn't known anymore and
survives inside old messages — the preview shows it.

**Invariant guard:** a test walks the source for log field keys (`.Str("…"`, `.Strs("…"`, …) and
fails when a key isn't classified in the redactor's table (keep / drop / pseudonymize / title).
That way a new log field can't leak by default — whoever adds it has to decide.

**Known limit:** free text inside `error` values is only scrubbed for the strings known from the
config. A title inside, say, a filesystem error survives with the titles checkbox off. The preview
shows it, and that's the honest answer for an edge case this small.

## Crash capture

Today a panic goes to stderr and is lost: it never reaches `daemon.log`, and on Windows (no console)
stderr goes nowhere. And when the daemon is down, so is the UI — the moment the user most wants to
report is exactly when the report screen can't open.

1. At boot, in `src/cmd/daemon/main.go`, before anything else that can panic: if
   `<data dir>/crash.log` exists and is non-empty, rename it to `crash-pending.log`. If an older
   `crash-pending.log` is still there (crashed again before anyone opened the UI), that one moves to
   the history first (step 5) — the latest crash is the pending one, none is lost.
2. Open a fresh `crash.log` and hand it to `debug.SetCrashOutput` (Go ≥ 1.23; the module is on
   1.25). The runtime writes the fatal panic / fatal error there **in addition to** stderr.
3. `/api/v1/status` gains `pending_crash_at` — the crash file's modification time, or `null`.
4. The UI reacts in two ways, depending on whether the page was just loaded:
   - **fresh page load** with `pending_crash_at` set → navigates to `#/report?crash=1`
   - **tab already open** when the value appears or changes → a persistent error toast ("the app
     stopped unexpectedly and restarted") with the Report link. This is the common case on a
     systemd service, which restarts the daemon on its own while the tab stays open — and forcing a
     route change there could throw away an unsaved Config form.

   The last `pending_crash_at` the UI reacted to is remembered (`localStorage`), so the same crash
   doesn't come back on every reload.
5. After **Open GitHub issue**, **Download report** or an explicit **Dismiss**, the UI calls
   `DELETE /api/v1/diagnostics/crash`, which moves `crash-pending.log` to
   `<data dir>/crashes/crash-<yyyymmdd-hhmmss>.log` (the crash time, from the file's modification
   time) and deletes all but the **5 newest** files there.

   The `crashes/` folder is the whole history feature: no screen lists it (decided 2026-10-14) —
   whoever wants old crashes opens the folder. `<data dir>` is `~/.autoAnimeDownloader` on Linux,
   `%APPDATA%\.autoAnimeDownloader` on Windows and `/app/data/.autoAnimeDownloader` in the docker
   image.

**No browser auto-open for this** (decided 2026-10-14). The screen appears the first time the user
opens the web app by hand after the crash. Opening a tab on its own at boot is invasive on a systemd
service, a headless box or docker — the cases where the app runs unattended.

What this doesn't catch: `kill -9` and the OOM killer (no Go code runs), and a daemon that crashes
**during boot**, every boot — it never serves the UI. That one is what a CLI command would cover
(see below).

## Code layout

**`src/internal/diagnostics`** builds the bundle **reading files from disk** — `config.json`,
`anime_settings.json`, `daemon.log` + backups, `crash-pending.log` — and never asks the running
daemon for anything. That's the decision that matters now: it's what lets a future
`autoanimedownloader report` CLI command call the same function with the daemon dead. The command
itself is left for `v2.6.0` (CLI parity) — a crash on every boot is rare enough to wait.

API, in `src/internal/api/endpoint_diagnostics.go`:

- `GET /api/v1/diagnostics?titles=true|false&error=…&context=…&anime=…` →
  `{ "bundle": "<text>", "environment": "<short summary for the issue URL>" }`
- `DELETE /api/v1/diagnostics/crash` → `204`

The frontend composes the rest (user text + bundle for the file; title + text +
`environment` for the issue URL).

## GitHub issue form

New `.github/ISSUE_TEMPLATE/bug_report.yml` (there is no `ISSUE_TEMPLATE` today), fields with
stable `id`s so the report screen can pre-fill them by query string:

- `what-happened` — textarea, required
- `environment` — textarea, pre-filled with the summary
- `install-method` — dropdown: AUR, Docker, Windows installer, release binary, built from source
- `attachment-check` — checkbox "I attached the report file (or I couldn't open the report screen)"

Pre-fill by field `id` is supported by GitHub issue forms; confirm the exact behavior (encoding,
dropdown pre-fill) when implementing. The URL is kept under ~6 KB by truncating `what-happened`
in the URL only — the full text is in the file anyway.

## Fixes bundled with this work

Found while designing this; fixed as part of the implementation.

1. **The automatic error toasts are invisible today.** There are two toast stores.
   `lib/stores/toast.ts` is the one `Toasts.svelte` renders (and it already supports a `link`).
   `lib/stores/toasts.ts` is the one `lib/api/client.ts:112` pushes its **automatic** API error
   toasts into, and **nothing renders it** — every non-`silent` API failure the client reports is
   silently dropped. Fix first, before the report screen: `client.ts` moves to `toast.ts`,
   `toasts.ts` is deleted, and `toast.error` gains the optional `link` that `toast.success` already
   has. Without this, the most useful entry point (an error the app itself caught) never shows up.
2. **The Logs screen's copy button fails over the LAN.** `routes/Logs.svelte:182` only calls
   `navigator.clipboard.writeText`, which doesn't exist outside a secure context — and the UI is
   routinely reached over plain http from another machine on the LAN (the real log shows
   `192.168.0.100`). It always lands in `logs_copy_error`. Fix: when `navigator.clipboard` is
   missing, fall back to selecting the text so the user can press Ctrl+C, and say so.
3. **Broken links in the MyAnimeList spec.**
   `2026-10-12-myanimelist-list-source-design.md` has 7 links to `../agents/…`; from
   `docs/superpowers/specs/` it has to be `../../agents/`.
4. **Portuguese leftovers** (the "Everything in English" rule):
   - spec filenames: `2026-08-10-limite-episodios-batch`, `2026-08-13-batch-por-filtro`,
     `2026-08-16-fontes-multiplas-etapa0`, `2026-08-24-filtro-idioma-legenda`,
     `2026-08-28-lista-de-arquivos-do-torrent`, `2026-08-28-presets-de-prioridade`,
     `2026-09-05-cache-local-anilist`, `2026-09-13-fonte-como-descriptor` — rename and update every
     link to them (`grep -rn` over the repo)
   - the `decisions.md` headings that `docs/TODO.md` links to (#73, #77, #84, #85, #94) — translate
     the headings and update **every** anchor that points to them, not just the TODO's
5. **`config.md` gives the wrong Windows path.** `docs/agents/config.md:3` says
   `%APPDATA%\AutoAnimeDownloader\config.json`; the code uses `%APPDATA%\.autoAnimeDownloader`
   (leading dot, `configsFolder` in `filemanager.go:16`), and `architecture.md:65` already says
   there is no dotless variant. Fix the line.
6. **The data dir path is computed in six places**, each with its own copy of
   `if windows → APPDATA else HOME` + `.autoAnimeDownloader`:
   `logger.go` (`getLogDirectory`), `files/filemanager.go` (`ensureConfigsFolder`),
   `cmd/daemon/main.go` (`getJobsFilePath`, `getSessionDBPath`, `getPIDFilePath`) and
   `internal/cli/process.go` (`getPIDFilePath`). This work would add a seventh (`crashes/`) and an
   eighth (`diagnostics`). Fix: one function in a new **leaf** package `src/internal/datadir`
   (`datadir.Path()`, creates the folder, returns it) and every site calls it. It has to be a leaf:
   `files` imports `logger`, so the function can't live in `files` — `logger` would have to import
   it back. Update `architecture.md:65`, which names the old functions.

## Not doing

- a backend / anonymous form (above)
- automatic upload of anything
- anything AI-related — a "Copy for AI" button was designed and dropped on 2026-10-13 by the
  user's call; the downloaded file is plain text anyone can hand to an AI
- machine specs beyond OS, distro, desktop and session
- the CLI `report` command — `v2.6.0`
- auto-opening the browser after a crash

## Docs to update when implementing

- `architecture.md`: the `diagnostics` package and why it reads from disk; the crash file
  lifecycle (`crash.log` → `crash-pending.log` → `crashes/`, newest 5 kept); the endpoint rows
- `decisions.md` #102: no backend — the user attaches the file, consent is the act of attaching
- `decisions.md` #103: redaction by key with a test that forces every log key to be classified
- `testing.md` if the key-classification test needs a note on how to classify a new key

## Testing

- `diagnostics`: redaction table — each level with a real log line from the current format; the
  webhook URL inside an `error` string; a URL to an unknown host inside `error` (a rotated webhook); usernames inside a message; titles off keeps the same
  `<title-N>` for the same anime across lines; the `.gz` fallback after rotation; docker/systemd
  detection with a fake filesystem/env
- the key-classification guard test
- the crash file rename at boot (pending exists / doesn't / empty `crash.log`); the move to
  `crashes/` and the prune to 5
- the endpoint: params passed through, `DELETE` idempotent
