# Releasing

How a version goes from "implemented" on `master` to a GitHub Release and an AUR bump.

The number comes from [TODO.md](../TODO.md): each section is one release, and its bump is the
largest bump of the tasks inside it. A task that does not touch the binary rides along without
changing the number. Patch items under "Fixes with no fixed release" ship as `vX.Y.Z` whenever
they are tackled.

Pushing a `v*` tag is what starts [`.github/workflows/release.yml`](../../.github/workflows/release.yml).
The workflow publishes the GitHub Release; there is nothing to click in the GitHub UI.

## 1. Mark the TODO

On `master`, with a clean tree and the work already committed:

1. Set `Current version:` to the new tag (`v2.6.0`).
2. Rewrite the section's item as **released in `v2.6.0` on YYYY-MM-DD**, same shape as `v2.5.0`.
3. Commit (subject line only, Conventional Commits, English):

```
docs: mark the cli parity as released in v2.6.0
```

## 2. Tag

Lightweight tag on that commit, matching the TODO:

```bash
git tag v2.6.0
git push origin master
git push origin v2.6.0
```

The tag is `v` plus the three-part number. If the stamp commit is wrong, make a new commit and tag
that one.

## 3. Wait for GitHub Actions

The `Release` workflow:

- builds Linux amd64, Linux arm64, and Windows
- commits `chore: update download links to vX.Y.Z` on `master` when the README asset names change
- publishes the GitHub Release with the zips, the Windows exe, their `.sha256` files, and generated notes

Step 4 reads those `.sha256` files. Start it only after the release exists and the checksum URLs
return 200.

## 4. Bump AUR `autoanimedownloader-bin`

`autoanimedownloader-bin` repackages the GitHub zips. It does not update itself.

The AUR clone is `ssh://aur@aur.archlinux.org/autoanimedownloader-bin.git`. A helper **outside this
repo**, next to it (`../update-bin.sh`), fills `pkgver`, resets `pkgrel` to 1, and writes
`sha256sums_x86_64` / `sha256sums_aarch64` from the GitHub `.sha256` assets into `../aad-bin/`.

```bash
../update-bin.sh 2.6.0
cd ../aad-bin
git add PKGBUILD .SRCINFO
git commit -m "update to v2.6.0"
git push
```

Passing the version (no `v` prefix) is the safe form when two tags go out close together. With no
argument the script uses GitHub `releases/latest`. The source URLs already interpolate `pkgver`.

`autoanimedownloader-git` computes `pkgver()` from `git describe` at build time. Touch that
PKGBUILD only when the package recipe itself changes.

## Out of this flow

Tracked elsewhere, not part of the stamp:

- ghcr image — `v3.0.0` in TODO.md; the Dockerfile is built in CI and never pushed. See
  [distribution.md](../distribution.md)
- Windows installer / service — `v3.1.0`
- CLI inside the Windows package — `package-windows` does not ship it
  ([architecture.md](../agents/architecture.md) `cli/`)
