#!/usr/bin/env bash
# Installs (or updates) the latest release for the current user: binaries in ~/.local/bin and the
# systemd user service, through the `make install-user` shipped inside the release zip.
#
#   curl -fsSL https://raw.githubusercontent.com/icarosuper/AutoAnimeDownloader/master/install.sh | bash
#
# Running it again updates: install-user stops the service, replaces the binaries and starts it.
# The asset is picked by its suffix, not by its full name, so the project prefix in the zip name can
# change without breaking this script. The checksum check is mandatory: this is curl into bash.
set -euo pipefail

REPO="icarosuper/AutoAnimeDownloader"

die() {
	echo "error: $*" >&2
	exit 1
}

[ "$(uname -s)" = "Linux" ] || die "only Linux is supported by this script (Windows: download the .exe from the releases page)"

for bin in curl unzip sha256sum make; do
	command -v "$bin" >/dev/null 2>&1 || die "'$bin' is required but not installed"
done

case "$(uname -m)" in
x86_64 | amd64) ARCH=Linux_x86 ;;
aarch64 | arm64) ARCH=Linux_Arm64 ;;
*) die "unsupported architecture: $(uname -m)" ;;
esac

RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest") ||
	die "failed to query the latest release from GitHub"

ZIP_URL=$(printf '%s\n' "$RELEASE_JSON" |
	sed -n 's/.*"browser_download_url": *"\([^"]*_'"$ARCH"'_v[^"]*\.zip\)".*/\1/p' | head -n 1)
[ -n "$ZIP_URL" ] || die "no $ARCH zip found in the latest release"

ZIP=$(basename "$ZIP_URL")
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $ZIP..."
curl -fsSL -o "$TMP/$ZIP" "$ZIP_URL" || die "failed to download $ZIP_URL"
curl -fsSL -o "$TMP/$ZIP.sha256" "$ZIP_URL.sha256" || die "failed to download $ZIP_URL.sha256"

(cd "$TMP" && sha256sum -c --quiet "$ZIP.sha256") || die "checksum mismatch for $ZIP, aborting"
unzip -q "$TMP/$ZIP" -d "$TMP"

PKG_DIR="$TMP/${ZIP%.zip}"
[ -f "$PKG_DIR/Makefile" ] || die "unexpected zip layout: $PKG_DIR/Makefile not found"

make --no-print-directory -C "$PKG_DIR" install-user

case ":$PATH:" in
*":$HOME/.local/bin:"*) ;;
*) echo "note: $HOME/.local/bin is not in your PATH; add it to use the CLI" ;;
esac
