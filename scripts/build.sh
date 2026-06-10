#!/bin/bash
# Builds a single platform via Docker and extracts the binaries.
# Usage: scripts/build.sh <platform> <version>
# Platforms: linuxamd64 | linuxarm64 | windows

set -e

PLATFORM=$1
VERSION=$2

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

if [ -z "$PLATFORM" ]; then
    echo -e "${RED}Usage: $0 <platform> <version>${NC}" >&2
    exit 1
fi

case "$PLATFORM" in
    linuxamd64)
        DOCKERFILE=docker/Dockerfile.build.amd64
        BUILD_DIR=build/linux-amd64
        DOCKER_IMAGE=aad-build-amd64
        CONTAINER=aad-temp-amd64
        ;;
    linuxarm64)
        DOCKERFILE=docker/Dockerfile.build.arm64
        BUILD_DIR=build/linux-arm64
        DOCKER_IMAGE=aad-build-arm64
        CONTAINER=aad-temp-arm64
        ;;
    windows)
        DOCKERFILE=docker/Dockerfile.build.windows
        BUILD_DIR=build/windows-amd64
        DOCKER_IMAGE=aad-build-windows
        CONTAINER=aad-temp-windows
        ;;
    *)
        echo -e "${RED}Unknown platform: $PLATFORM${NC}" >&2
        echo -e "${YELLOW}Valid platforms: linuxamd64 linuxarm64 windows${NC}" >&2
        exit 1
        ;;
esac

if ! command -v docker >/dev/null 2>&1; then
    echo -e "${RED}Error: Docker is not installed or not in PATH${NC}" >&2
    exit 1
fi

mkdir -p "$BUILD_DIR"

_has_buildx() {
    docker buildx version >/dev/null 2>&1
}

_build_with_buildx() {
    echo -e "${YELLOW}Using docker buildx...${NC}"
    local extra_flags=()
    [ "$PLATFORM" = "linuxarm64" ] && extra_flags+=(--platform linux/arm64)
    docker buildx build \
        "${extra_flags[@]}" \
        --load \
        -f "$DOCKERFILE" \
        --build-arg VERSION="$VERSION" \
        -t "$DOCKER_IMAGE" \
        . || exit 1
}

_build_classic() {
    echo -e "${YELLOW}Using docker build (faster for local builds)...${NC}"
    docker build -f "$DOCKERFILE" --build-arg VERSION="$VERSION" -t "$DOCKER_IMAGE" .
}

_extract_binaries() {
    echo -e "${YELLOW}Extracting binaries...${NC}"
    docker rm -f "$CONTAINER" 2>/dev/null || true
    docker create --name "$CONTAINER" "$DOCKER_IMAGE"
    docker cp "$CONTAINER":/output/. "$BUILD_DIR"/
    docker rm "$CONTAINER"
}

_generate_checksums() {
    cd "$BUILD_DIR"
    if [ "$PLATFORM" = "windows" ]; then
        sha256sum autoanimedownloader-daemon.exe > autoanimedownloader-daemon.exe.sha256 2>/dev/null || true
        sha256sum autoanimedownloader.exe > autoanimedownloader.exe.sha256 2>/dev/null || true
    else
        sha256sum autoanimedownloader-daemon > autoanimedownloader-daemon.sha256 2>/dev/null || true
        sha256sum autoanimedownloader > autoanimedownloader.sha256 2>/dev/null || true
    fi
    cd - > /dev/null
}

# CI/CD always uses buildx; ARM64 requires it; everything else prefers classic locally
if [ -n "$CI" ] || [ -n "$GITHUB_ACTIONS" ]; then
    if _has_buildx; then
        echo -e "${YELLOW}CI/CD detected — using buildx...${NC}"
        _build_with_buildx
    else
        _build_classic
    fi
elif [ "$PLATFORM" = "linuxarm64" ]; then
    if _has_buildx; then
        _build_with_buildx
    else
        echo -e "${RED}Error: docker buildx is required for ARM64 builds${NC}" >&2
        echo -e "${YELLOW}Install with: docker buildx install${NC}" >&2
        exit 1
    fi
else
    _build_classic
fi

_extract_binaries
_generate_checksums

echo -e "${GREEN}✓ $PLATFORM build complete: $BUILD_DIR${NC}"
