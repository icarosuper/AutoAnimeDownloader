#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Packaging AutoAnimeDownloader for Linux...${NC}"

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# Check if build directory exists
if [ ! -d "build" ]; then
    echo -e "${RED}Error: build directory not found. Run scripts/build.sh first.${NC}"
    exit 1
fi

# Create packages directory inside build
PACKAGES_DIR="$PROJECT_ROOT/build/packages"
mkdir -p "$PACKAGES_DIR"

# Create temporary packaging directory
PACKAGE_DIR=$(mktemp -d)
trap "rm -rf $PACKAGE_DIR" EXIT

echo -e "${YELLOW}Creating packages...${NC}"

# Package for amd64
if [ -d "build/linux-amd64" ]; then
    echo -e "${YELLOW}Packaging Linux amd64...${NC}"
    PACKAGE_NAME="AutoAnimeDownloader_Linux_x86"
    PACKAGE_DIR_ARCH="$PACKAGE_DIR/$PACKAGE_NAME"
    mkdir -p "$PACKAGE_DIR_ARCH"
    
    # Copy binaries
    cp build/linux-amd64/AutoAnimeDownloader-daemon "$PACKAGE_DIR_ARCH/"
    cp build/linux-amd64/AutoAnimeDownloader-cli "$PACKAGE_DIR_ARCH/"
    
    # Copy service file and Makefile
    cp infra/linux/autoanimedownloader.service "$PACKAGE_DIR_ARCH/"
    cp infra/linux/Makefile "$PACKAGE_DIR_ARCH/"
    
    # Create README
    cat > "$PACKAGE_DIR_ARCH/README.md" << 'EOF'
# AutoAnimeDownloader - Linux Installation

## Installation

1. Extract this archive
2. Run `make install` in the extracted directory

This will install:
- AutoAnimeDownloader-daemon (with embedded frontend)
- AutoAnimeDownloader-cli
- systemd user service

## Usage

After installation, the daemon will start automatically. You can:

- Access the web UI at http://localhost:8091
- Use the CLI: `AutoAnimeDownloader-cli status`
- Manage the service: `systemctl --user status autoanimedownloader`

## Uninstallation

Run `make uninstall` in the installation directory.
EOF
    
    # Create ZIP
    cd "$PACKAGE_DIR"
    zip -r "$PACKAGES_DIR/$PACKAGE_NAME.zip" "$PACKAGE_NAME" > /dev/null
    
    # Generate checksum
    cd "$PACKAGES_DIR"
    sha256sum "$PACKAGE_NAME.zip" > "$PACKAGE_NAME.zip.sha256"
    
    echo -e "${GREEN}Created: build/packages/$PACKAGE_NAME.zip${NC}"
    
    # Return to project root for next package
    cd "$PROJECT_ROOT"
fi

# Package for arm64
if [ -d "build/linux-arm64" ]; then
    echo -e "${YELLOW}Packaging Linux arm64...${NC}"
    PACKAGE_NAME="AutoAnimeDownloader_Linux_Arm64"
    PACKAGE_DIR_ARCH="$PACKAGE_DIR/$PACKAGE_NAME"
    mkdir -p "$PACKAGE_DIR_ARCH"
    
    # Copy binaries
    cp build/linux-arm64/AutoAnimeDownloader-daemon "$PACKAGE_DIR_ARCH/"
    cp build/linux-arm64/AutoAnimeDownloader-cli "$PACKAGE_DIR_ARCH/"
    
    # Copy service file and Makefile
    cp infra/linux/autoanimedownloader.service "$PACKAGE_DIR_ARCH/"
    cp infra/linux/Makefile "$PACKAGE_DIR_ARCH/"
    
    # Create README
    cat > "$PACKAGE_DIR_ARCH/README.md" << 'EOF'
# AutoAnimeDownloader - Linux ARM64 Installation

## Installation

1. Extract this archive
2. Run `make install` in the extracted directory

This will install:
- AutoAnimeDownloader-daemon (with embedded frontend)
- AutoAnimeDownloader-cli
- systemd user service

## Usage

After installation, the daemon will start automatically. You can:

- Access the web UI at http://localhost:8091
- Use the CLI: `AutoAnimeDownloader-cli status`
- Manage the service: `systemctl --user status autoanimedownloader`

## Uninstallation

Run `make uninstall` in the installation directory.
EOF
    
    # Create ZIP
    cd "$PACKAGE_DIR"
    zip -r "$PACKAGES_DIR/$PACKAGE_NAME.zip" "$PACKAGE_NAME" > /dev/null
    
    # Generate checksum
    cd "$PACKAGES_DIR"
    sha256sum "$PACKAGE_NAME.zip" > "$PACKAGE_NAME.zip.sha256"
    
    echo -e "${GREEN}Created: build/packages/$PACKAGE_NAME.zip${NC}"
    
    # Return to project root
    cd "$PROJECT_ROOT"
fi

echo -e "${GREEN}Packaging complete!${NC}"
echo -e "Packages are in: ${GREEN}build/packages/${NC}"

