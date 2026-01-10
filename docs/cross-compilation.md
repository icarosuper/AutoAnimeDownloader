# Cross-Compilation com CGO

Este documento explica como configurar cross-compilation com CGO para compilar o AutoAnimeDownloader com suporte a tray icon para todas as plataformas.

## Visão Geral

**Importante**: Cross-compilation com CGO é muito complexa porque requer não apenas o toolchain cross-compiler, mas também todas as bibliotecas GTK/AppIndicator para a arquitetura alvo. No Arch Linux e muitas outras distribuições, essas bibliotecas não estão disponíveis para ARM64.

**Solução Recomendada**: Use Docker para builds cross-platform com tray icon. O Docker fornece um ambiente isolado com todas as dependências necessárias.

**Solução Alternativa**: O script de build detecta automaticamente e desabilita CGO para cross-compilation local. Apenas builds nativos (amd64 → amd64 ou arm64 → arm64) terão tray icon quando compilados localmente sem Docker.

## Build Automático

Use o Makefile (recomendado) ou scripts como fallback:

```bash
# Método recomendado
make build

# Fallback (se make não estiver disponível)
./scripts/build/build-all.sh

# Build para plataforma específica
make build-linux-amd64
make build-linux-arm64
make build-windows

# Ou usando scripts individuais
./scripts/build/build-linux-amd64.sh
./scripts/build/build-linux-arm64.sh
./scripts/build/build-windows.sh
```

**Como funciona:**
- Todos os builds usam Docker para garantir tray icon em todas as plataformas
- Docker compila nativamente em containers para cada arquitetura
- Builds consistentes e funcionais

O Docker automaticamente:
- Compila nativamente em containers ARM64 para ARM64
- Instala todas as dependências necessárias
- Gera binários com tray icon para todas as plataformas

## Instalação Manual (Apenas para Builds Nativos)

Se você estiver compilando nativamente (mesma arquitetura), instale apenas as dependências do tray icon:

### Arch Linux / Manjaro / CachyOS

```bash
sudo pacman -S libayatana-appindicator
```

### Ubuntu / Debian

```bash
sudo apt-get install -y libayatana-appindicator3-dev
```

### Fedora

```bash
sudo dnf install -y libappindicator-gtk3-devel
```

**Nota**: Para cross-compilation, use Docker. Não é necessário instalar toolchains cross-compiler manualmente.

## Como Funciona

### Com Docker (Recomendado)

Quando você usa `make build` ou `./scripts/build/build-all.sh`:

1. **AMD64**: Compila em container AMD64 nativo com CGO=1 e tray icon ✓
2. **ARM64**: Compila em container ARM64 nativo com CGO=1 e tray icon ✓
3. **Windows**: Compila usando mingw-w64 no Docker com CGO=1 e tray icon ✓
4. Todas as dependências estão disponíveis nos containers
5. Builds consistentes e funcionais

**Nota**: Todos os builds cross-platform usam Docker para garantir tray icon. Builds locais sem Docker só funcionam para a arquitetura nativa e requerem dependências instaladas localmente.

## Verificação

Para verificar se as dependências do tray icon estão instaladas (apenas para builds nativos):

```bash
# Verificar dependências do tray icon
pkg-config --modversion ayatana-appindicator3-0.1
```

## GitHub Actions

Os workflows do GitHub Actions já estão configurados para:

- Usar Docker para builds ARM64 com tray icon
- Compilar com CGO habilitado para Windows (nativo)
- Compilar com CGO habilitado para Linux AMD64 (nativo)
- Incluir tray icon em todos os builds

Não é necessária nenhuma ação adicional - os builds na pipeline já funcionam com tray icon.

## Troubleshooting

### Build sem tray icon

Se o build não incluir tray icon:

1. **Para builds nativos**: Verifique se as dependências do tray icon estão instaladas (`pkg-config --modversion ayatana-appindicator3-0.1`)
2. **Para cross-compilation**: Docker é usado automaticamente pelo `make build` ou scripts
3. Verifique se Docker está instalado e rodando: `docker --version`

## Notas

- O CLI sempre compila sem CGO (não precisa de tray icon)
- Apenas o daemon precisa de CGO para tray icon
- Builds cross-compilation locais ainda funcionam, mas sem tray icon (use Docker para ter tray icon)
- Docker é o método recomendado para builds cross-platform com tray icon

