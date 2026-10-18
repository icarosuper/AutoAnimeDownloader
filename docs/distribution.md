# Distribution / installation

How the app is installed today and what is worth adding. Ordered by return on effort.

## Today

- **AUR** — `autoanimedownloader-bin` repackages the release zips and gets its `pkgver` and
  checksums bumped by hand on every release (helper outside the repo);
  `autoanimedownloader-git` builds from `master` and needs no bump. The stamp steps, including
  this bump, are in [guides/release.md](guides/release.md)
- **GitHub Releases zip** + `make install-user` / `sudo make install-global` (`infra/linux/Makefile`)
- **Windows** — a standalone exe, runs in the foreground, no service and no installer

None of these gives **automatic updates**. That's the main gap, not the lack of a one-liner.

## 1. Docker image on ghcr — biggest win

`docker/Dockerfile` already exists and is already built in CI (the integration job in `build.yml`),
but it's never published. For a headless daemon with a web UI (same shape as Sonarr/Radarr), the
homelab/NAS audience expects `docker compose up -d`.

Cost: ~15 lines in `.github/workflows/release.yml`, no new file, no new script to maintain.

```yaml
  release-docker:
    needs: build-frontend
    runs-on: ubuntu-latest
    permissions: { contents: read, packages: write }
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: ${{ github.actor }}, password: ${{ secrets.GITHUB_TOKEN }} }
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: docker/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: |
            ghcr.io/icarosuper/autoanimedownloader:${{ needs.build-frontend.outputs.version }}
            ghcr.io/icarosuper/autoanimedownloader:latest
```

Win: installing, updating (`docker compose pull`) and uninstalling become one command, on any
distro, Synology, unRAID, TrueNAS. All that's missing is a README section with a ~10-line compose
file (a volume for `completed_anime_path` and one for the data, port 8091).

## 2. `install.sh` (curl | bash) — second step

Serves whoever runs bare-metal outside Arch. ~30 lines at the repo root, reusing the existing
`make install-user`:

```bash
curl -fsSL https://raw.githubusercontent.com/icarosuper/AutoAnimeDownloader/master/install.sh | bash
```

```bash
#!/usr/bin/env bash
set -euo pipefail
case "$(uname -m)" in
  x86_64) ASSET=Linux_x86 ;;
  aarch64|arm64) ASSET=Linux_Arm64 ;;
  *) echo "unsupported arch: $(uname -m)"; exit 1 ;;
esac
TAG=$(curl -fsSL https://api.github.com/repos/icarosuper/AutoAnimeDownloader/releases/latest | grep -oP '"tag_name":\s*"\K[^"]+')
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
ZIP="AutoAnimeDownloader_${ASSET}_${TAG}.zip"
curl -fsSL -o "$TMP/$ZIP"        "https://github.com/icarosuper/AutoAnimeDownloader/releases/download/$TAG/$ZIP"
curl -fsSL -o "$TMP/$ZIP.sha256" "https://github.com/icarosuper/AutoAnimeDownloader/releases/download/$TAG/$ZIP.sha256"
(cd "$TMP" && sha256sum -c "$ZIP.sha256" && unzip -q "$ZIP")
make -C "$TMP"/AutoAnimeDownloader_* install-user
```

Checking the `.sha256` is **not optional** — it's curl straight into bash. The checksum files are
already published by `release.yml`. Still no automatic updates: whoever installed this way runs
the script again.

## Evaluated and discarded

- **`go install`** — doesn't work. `dist/` is gitignored and the `//go:embed dist/*`
  (`src/internal/frontend/embed.go`) breaks without the bun build first. It would only work by
  committing `dist/`, which isn't worth it.
- **Homebrew** — there's no macOS build. Nothing to gain today.
- **Scoop / Winget** — Scoop is a JSON file in a bucket, cheap, but Windows today is a foreground
  exe with no service. It solves the wrong problem first; see the "proper Windows release" section
  in the TODO.
- **.deb / .rpm (nfpm)** — covers Debian/Ubuntu/Fedora with `apt install ./x.deb`, but without a
  hosted repo there are no automatic updates either. Docker covers that audience with less work.
- **Flatpak / Snap / Nix** — disproportionate weight for a daemon.
