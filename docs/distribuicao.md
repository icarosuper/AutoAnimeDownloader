# Distribuição / instalação

Como o app é instalado hoje e o que vale adicionar. Ordenado por retorno sobre esforço.

## Hoje

- **AUR** (`autoanimedownloader-bin`) — `scripts/update-aur.sh` atualiza os PKGBUILDs a cada release
- **Zip do GitHub Releases** + `make install-user` / `sudo make install-global` (`infra/linux/Makefile`)
- **Windows** — exe solto, roda em foreground, sem serviço nem instalador

Nenhum desses dá **update automático**. Esse é o buraco principal, não a falta de um one-liner.

## 1. Imagem Docker no ghcr — maior ganho

`docker/Dockerfile` já existe e já é buildado no CI (job de integração do `build.yml`), mas
nunca é publicado. Para um daemon headless com web UI (mesmo formato de Sonarr/Radarr), o
público de homelab/NAS espera `docker compose up -d`.

Custo: ~15 linhas em `.github/workflows/release.yml`, nenhum arquivo novo, nenhum script novo
para manter.

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

Ganho: instalar, atualizar (`docker compose pull`) e desinstalar viram um comando, em qualquer
distro, Synology, unRAID, TrueNAS. Falta só uma seção no README com um compose de ~10 linhas
(volume para `completed_anime_path` e para os dados, porta 8091).

## 2. `install.sh` (curl | bash) — segundo passo

Atende quem roda bare-metal fora do Arch. ~30 linhas na raiz do repo, reusando o
`make install-user` que já existe:

```bash
curl -fsSL https://raw.githubusercontent.com/icarosuper/AutoAnimeDownloader/master/install.sh | bash
```

```bash
#!/usr/bin/env bash
set -euo pipefail
case "$(uname -m)" in
  x86_64) ASSET=Linux_x86 ;;
  aarch64|arm64) ASSET=Linux_Arm64 ;;
  *) echo "arch não suportada: $(uname -m)"; exit 1 ;;
esac
TAG=$(curl -fsSL https://api.github.com/repos/icarosuper/AutoAnimeDownloader/releases/latest | grep -oP '"tag_name":\s*"\K[^"]+')
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
ZIP="AutoAnimeDownloader_${ASSET}_${TAG}.zip"
curl -fsSL -o "$TMP/$ZIP"        "https://github.com/icarosuper/AutoAnimeDownloader/releases/download/$TAG/$ZIP"
curl -fsSL -o "$TMP/$ZIP.sha256" "https://github.com/icarosuper/AutoAnimeDownloader/releases/download/$TAG/$ZIP.sha256"
(cd "$TMP" && sha256sum -c "$ZIP.sha256" && unzip -q "$ZIP")
make -C "$TMP"/AutoAnimeDownloader_* install-user
```

Conferir o `.sha256` **não é opcional** — é curl direto para bash. Os arquivos de checksum já
são publicados pelo `release.yml`. Continua sem update automático: quem instalou assim roda o
script de novo.

## Avaliados e descartados

- **`go install`** — não funciona. `dist/` é gitignored e o `//go:embed dist/*`
  (`src/internal/frontend/embed.go`) quebra sem o build do bun antes. Só passaria a funcionar
  commitando o `dist/`, o que não compensa.
- **Homebrew** — não há build de macOS. Nada a ganhar hoje.
- **Scoop / Winget** — Scoop é um JSON num bucket, barato, mas o Windows hoje é um exe em
  foreground sem serviço. Resolve o problema errado primeiro; ver o item "Proper release no
  Windows" no TODO.
- **.deb / .rpm (nfpm)** — cobre Debian/Ubuntu/Fedora com `apt install ./x.deb`, mas sem repo
  hospedado também não dá update automático. Docker cobre esse público com menos trabalho.
- **Flatpak / Snap / Nix** — peso desproporcional para um daemon.
