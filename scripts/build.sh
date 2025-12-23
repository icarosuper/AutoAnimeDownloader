#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building AutoAnimeDownloader for Linux...${NC}"

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# Create build directories
mkdir -p build/linux-amd64
mkdir -p build/linux-arm64

echo -e "${YELLOW}Step 1: Building frontend...${NC}"
cd src/internal/frontend
npm ci
npm run build
cd "$PROJECT_ROOT"

echo -e "${YELLOW}Step 2: Building daemon for Linux amd64...${NC}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-amd64/autoanimedownloader-daemon ./src/cmd/daemon

echo -e "${YELLOW}Step 3: Building daemon for Linux arm64...${NC}"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-arm64/autoanimedownloader-daemon ./src/cmd/daemon

echo -e "${YELLOW}Step 4: Building CLI for Linux amd64...${NC}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-amd64/autoanimedownloader ./src/cmd/cli

echo -e "${YELLOW}Step 5: Building CLI for Linux arm64...${NC}"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o build/linux-arm64/autoanimedownloader ./src/cmd/cli

echo -e "${YELLOW}Step 6: Generating checksums...${NC}"
cd build/linux-amd64
sha256sum autoanimedownloader-daemon > autoanimedownloader-daemon.sha256
sha256sum autoanimedownloader > autoanimedownloader.sha256
cd ../linux-arm64
sha256sum autoanimedownloader-daemon > autoanimedownloader-daemon.sha256
sha256sum autoanimedownloader > autoanimedownloader.sha256
cd "$PROJECT_ROOT"

echo -e "${GREEN}Build complete!${NC}"
echo -e "Binaries are in:"
echo -e "  - ${GREEN}build/linux-amd64/${NC}"
echo -e "  - ${GREEN}build/linux-arm64/${NC}"

