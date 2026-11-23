# TODO

Grouped by release: each section is **one** release, and its bump is the largest bump of the tasks
inside it — a task that doesn't touch the binary (docs, things outside the repo) rides along without
changing the number. Only what is still to ship lives here: a section is deleted when its release
ships (the GitHub Release and git history are the record), never kept struck through.
Current version: `v2.11.4`. Stamp a version (tag, GitHub Release, AUR):
the [release skill](../.claude/skills/release/SKILL.md).
Anything not tied to a specific release — problems and features alike — lives in
[backlog/](backlog/), one file per item.

The order follows a single rule: **nothing that stamps the project's name ships before the rename**
(Windows installer, ghcr image, README, social preview, landing page), otherwise it's a double
migration. If presenting the repo to employers gets urgent, `v3.0.0` can move up ahead of `v2.12.0`.

## `v2.12.0` — dashboard recents

A shelf of just-landed, still-unwatched episodes on Status, and last-check rows that open the
anime. Independent of the picker; does not wait for the `v3.0.0` UI overhaul.

- Recents shelf, clickable last-check rows. Spec designed on 2026-10-23:
  [dashboard-recent-shelf](superpowers/specs/2026-10-23-dashboard-recent-shelf-design.md).

## `v2.13.0` — episode torrent picker

- Pick torrent when downloading / redownloading a single episode — same screen as the
  whole-title picker in `v2.7.0`, singles search instead of packs. Own spec; do not start
  it in the same change as the dashboard recents.

## `v3.0.0` — Serval

Working checklist (one box per change, for coordinating chats): [v3.0.0.md](v3.0.0.md).

Everything that stamps the project's name and look, in a single release. The major comes from the
rename, and the Nyaa descriptor comes along so whoever upgrades goes through **one** migration, not
two.

- Rename the project to **Serval** — Go module name, binaries (`daemon`/`cli`), GitHub repo,
  `config.json` and the user data directory, ghcr image, webApp titles. Breaks the data
  path for existing users → needs a migration or an explicit release note
- **Take Nyaa out of the repository (source as an external descriptor)** — spec designed on
  2026-09-13 and never executed:
  [source-as-descriptor](superpowers/specs/2026-09-13-source-as-descriptor-design.md).
  The goal is **publishability**, not multiple sources: Nyaa's address, query and layout leave the
  binary for a JSON file the user puts next to `config.json`. A compiled Go plugin is out (the
  `plugin` package is Linux-only and requires the same compiler and the same deps). The processing
  does **not** go in the descriptor: of the 2217 lines in `internal/nyaa`, ~150 are Nyaa-specific
  and ~2050 are release-name parsing, the same for any source. The bulk of the diff is the split
  into `internal/release` + `internal/source` (21 files import the package, 277 occurrences of
  `nyaa.`), and steps 1-2 of the spec stand on their own — they can ship early in a minor, since
  they don't change behavior. Whoever upgrades without the descriptor is left with no source: the
  rename migration covers that
- UI/UX overhaul of the webApp
- Visual identity — logo, palette, favicon; feeds the social preview and the landing page
- Publish the image on ghcr — the biggest win among the options measured in
  [distribution.md](distribution.md), the Dockerfile already exists. The `curl` `install.sh` already
  shipped. `go install`, Homebrew, Scoop, .deb/.rpm and Flatpak were evaluated and discarded
- Update the README — after the image, because that's what changes the install instructions
- Upload a social preview on GitHub — today the shared link card is GitHub's generic one.
  Settings > General > Social preview > Upload, 1280x640, using the new visual identity. Web only,
  `gh repo edit` doesn't cover it
- Create a landing page

## `v3.1.0` — proper Windows release

After the rename: the installer, the service name and the data path stamp the name, and Windows UI
tweaks would be redone by the `v3.0.0` UI/UX overhaul.

- Authenticate the app with a Microsoft account
- Install instead of running from an executable file
- Detach and run as a background service
- UI changes to work better on Windows
- Test thoroughly
