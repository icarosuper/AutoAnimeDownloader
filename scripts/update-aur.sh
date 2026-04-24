#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Uso: $0 <versao>  ex: $0 1.3.0}"
AUR_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
BASE_URL="https://github.com/icarosuper/AutoAnimeDownloader/releases/download/v${VERSION}"

update_bin() {
  local pkgdir="$AUR_DIR/aad-bin"
  local x86_file="AutoAnimeDownloader_Linux_x86_v${VERSION}.zip"
  local arm_file="AutoAnimeDownloader_Linux_Arm64_v${VERSION}.zip"

  echo "Buscando checksums para aad-bin..."
  local sha_x86 sha_arm
  sha_x86=$(curl -fsSL "${BASE_URL}/${x86_file}.sha256" | awk '{print $1}')
  sha_arm=$(curl -fsSL "${BASE_URL}/${arm_file}.sha256" | awk '{print $1}')

  sed -i \
    -e "s/^pkgver=.*/pkgver=${VERSION}/" \
    -e "s/^pkgrel=.*/pkgrel=1/" \
    -e "s|AutoAnimeDownloader_Linux_x86_v[^']*\.zip|${x86_file}|g" \
    -e "s|AutoAnimeDownloader_Linux_Arm64_v[^']*\.zip|${arm_file}|g" \
    -e "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('${sha_x86}')/" \
    -e "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('${sha_arm}')/" \
    "$pkgdir/PKGBUILD"

  (cd "$pkgdir" && makepkg --printsrcinfo > .SRCINFO)
  (cd "$pkgdir" && git add PKGBUILD .SRCINFO && git commit -m "Release v${VERSION}" && git push)
  echo "aad-bin atualizado."
}

update_git() {
  local pkgdir="$AUR_DIR/aad-git"

  sed -i "s/^pkgver=.*/pkgver=${VERSION}.r0.g0000000/" "$pkgdir/PKGBUILD"

  (cd "$pkgdir" && makepkg --printsrcinfo > .SRCINFO)
  (cd "$pkgdir" && git add PKGBUILD .SRCINFO && git commit -m "Release v${VERSION}" && git push)
  echo "aad-git atualizado."
}

update_bin
update_git
