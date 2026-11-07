---
name: release
description: Ship an AutoAnimeDownloader version end to end — stamp TODO.md, tag and push, wait for the GitHub Release workflow, then bump the AUR package autoanimedownloader-bin. Use ONLY when the user explicitly asks to release, publish, tag or stamp a version (e.g. "faz a release da 2.9.0", "release v2.9.0", "/release 2.9.0"). Never use it on your own initiative, to "prepare" or "check" a release, or because a TODO section looks finished.
---

# Release a version

Takes the version as argument: `$ARGUMENTS` (e.g. `2.9.0` or `v2.9.0`). Below, `X.Y.Z` is the
number without `v`, `vX.Y.Z` the tag. No version given → read `docs/TODO.md`, propose the number
(rule below) and ask; never pick one silently.

This skill pushes to `origin` and to the AUR — things that can't be quietly undone. Being invoked
is **not** the permission: there are two confirmations below (before the GitHub push, before the
AUR push), and each one waits for an explicit yes in that message. Anything unexpected along the
way — dirty tree, failed workflow, missing checksum — stop and report; don't work around it.

## 0. Pre-flight (read-only)

```bash
git rev-parse --abbrev-ref HEAD          # must be master
git status --porcelain                   # must be empty
git fetch origin && git status -sb       # must not be behind origin/master
git tag -l "vX.Y.Z"                      # must be empty
go test ./...                            # must pass
```

Release from the **main checkout** on `master`, not from a worktree or a feature branch. If the
user has other work going on in that checkout, ask before touching it.

**The number** comes from `docs/TODO.md`: each section is one release, its bump is the largest bump
of the tasks inside it. A task that doesn't touch the binary rides along without changing the
number. Patch items under "Fixes with no fixed release" ship as `vX.Y.Z` whenever tackled. If the
requested number disagrees with the section, say so and ask.

## 1. Stamp the TODO

In `docs/TODO.md`:
1. `Current version:` → `` `vX.Y.Z` ``.
2. The section's item(s) rewritten as **released in `vX.Y.Z` on YYYY-MM-DD** (today), same shape
   as the previous release's.

Commit, subject line only (CLAUDE.md commit style):

```
docs: mark <what shipped> as released in vX.Y.Z
```

## 2. Confirmation #1 — then tag and push

Show the user: the stamp commit (`git show --stat HEAD`), the commits going out
(`git log --oneline origin/master..HEAD`) and the tag name. Ask: *"Push master and tag vX.Y.Z?"*
Wait for an explicit yes. On yes:

```bash
git tag vX.Y.Z                 # lightweight, on the stamp commit
git push origin master
git push origin vX.Y.Z
```

Pushing a `v*` tag is what starts `.github/workflows/release.yml`; there is nothing to click on
GitHub. A wrong stamp commit is fixed with a new commit and a new tag — never move or delete a
pushed tag without the user asking.

## 3. Wait for GitHub Actions

```bash
gh run list --workflow=release.yml --limit 1          # find the run for vX.Y.Z
gh run watch <run-id> --exit-status                   # run_in_background: true
```

The workflow builds Linux amd64, Linux arm64 and Windows; commits
`chore: update download links to vX.Y.Z` on `master` when the README asset names change; publishes
the Release with the zips, the Windows exe, their `.sha256` files and generated notes. Failed →
stop, show the failing job's log (`gh run view <run-id> --log-failed | tail -50`), and ask.

Then `git pull --ff-only origin master` (the workflow may have committed) and check the checksums
the AUR step reads:

```bash
for a in Linux_x86 Linux_Arm64; do
  curl -sfIL -o /dev/null -w "%{http_code} $a\n" \
    "https://github.com/icarosuper/AutoAnimeDownloader/releases/download/vX.Y.Z/AutoAnimeDownloader_${a}_vX.Y.Z.zip.sha256"
done
```

Both must be `200` before step 4.

## 4. Bump AUR `autoanimedownloader-bin`

The `-bin` package repackages the GitHub zips and doesn't update itself. Its clone and helper live
**outside this repo**, in the directory that contains the main checkout:

```bash
AAD_PARENT="$(dirname "$(dirname "$(realpath "$(git rev-parse --git-common-dir)")")")"
ls "$AAD_PARENT/update-bin.sh" "$AAD_PARENT/aad-bin/PKGBUILD"
```

Either missing → this machine can't do the AUR step: stop here and tell the user the GitHub part is
done. Otherwise:

```bash
"$AAD_PARENT/update-bin.sh" X.Y.Z      # always pass the version; no arg = releases/latest
git -C "$AAD_PARENT/aad-bin" diff
```

The helper sets `pkgver`, resets `pkgrel=1`, writes `sha256sums_x86_64`/`sha256sums_aarch64` from
the `.sha256` assets and regenerates `.SRCINFO` (needs `makepkg`). The source URLs already
interpolate `pkgver`. The diff must touch only those lines.

**Confirmation #2:** show the diff, ask *"Push the AUR bump to vX.Y.Z?"*, wait for an explicit yes.
On yes:

```bash
cd "$AAD_PARENT/aad-bin"
git add PKGBUILD .SRCINFO
git commit -m "update to vX.Y.Z"
git push
```

`autoanimedownloader-git` (`$AAD_PARENT/aad-git`) computes `pkgver()` from `git describe` at build
time — don't touch it unless the package recipe itself changes.

## Out of this flow

Tracked elsewhere, not part of a stamp: the ghcr image (`v3.0.0` in TODO.md; the Dockerfile is
built in CI and never pushed — see `docs/distribution.md`), the Windows installer/service
(`v3.1.0`), and the CLI inside the Windows package (`package-windows` doesn't ship it).

## Report

The tag, the Release URL (`gh release view vX.Y.Z --json url -q .url`), whether the workflow
committed the README links, and whether the AUR bump was pushed (or why not).
