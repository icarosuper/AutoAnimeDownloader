# Makefile for AutoAnimeDownloader
# Cross-platform builds using Docker
# Works on Linux, macOS, and WSL (Windows Subsystem for Linux)

.PHONY: build package release clean help dev clear-data debug-anime
.PHONY: build-linuxamd64 build-linuxarm64 build-windows
.PHONY: package-linuxamd64 package-linuxarm64 package-windows
.PHONY: release-linuxamd64 release-linuxarm64 release-windows
.PHONY: test test-backend test-backend-unit test-backend-integration
.PHONY: test-frontend test-frontend-unit test-frontend-component test-frontend-smoke

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m

# Get version from git tag or use default
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev")

# Platform support
PLATFORMS := linuxamd64 linuxarm64 windows
PLATFORM ?=

# Build output directories
BUILD_DIR_linuxamd64 := build/linux-amd64
BUILD_DIR_linuxarm64 := build/linux-arm64
BUILD_DIR_windows := build/windows-amd64

PACKAGE_NAME_linuxamd64 = AutoAnimeDownloader_Linux_x86_v$(VERSION)
PACKAGE_NAME_linuxarm64 = AutoAnimeDownloader_Linux_Arm64_v$(VERSION)
PACKAGE_NAME_windows = AutoAnimeDownloader_Windows_v$(VERSION)

PACKAGES_DIR := build/packages

ifeq ($(PLATFORM),)
  PLATFORMS_TO_PROCESS := $(PLATFORMS)
else
  PLATFORMS_TO_PROCESS := $(PLATFORM)
endif

help:
	@echo -e "$(GREEN)Build targets:$(NC)"
	@echo -e "  $(YELLOW)make build$(NC)                         - Build for all platforms"
	@echo -e "  $(YELLOW)make build PLATFORM=windows$(NC)        - Build for specific platform"
	@echo -e "  $(YELLOW)make package$(NC)                       - Package for all platforms"
	@echo -e "  $(YELLOW)make package PLATFORM=linuxamd64$(NC)   - Package for specific platform"
	@echo -e "  $(YELLOW)make release$(NC)                       - Build + package for all platforms"
	@echo -e "  $(YELLOW)make release PLATFORM=linuxarm64$(NC)   - Build + package for specific platform"
	@echo -e "  $(YELLOW)make clean$(NC)                         - Remove build artifacts"
	@echo ""
	@echo -e "$(GREEN)Test targets:$(NC)"
	@echo -e "  $(YELLOW)make test$(NC)                          - Run all suites with pass/fail summary"
	@echo -e "  $(YELLOW)make test-backend$(NC)                  - Run backend unit + integration"
	@echo -e "  $(YELLOW)make test-backend-unit$(NC)             - Run Go unit tests"
	@echo -e "  $(YELLOW)make test-backend-integration$(NC)      - Run integration tests (Docker)"
	@echo -e "  $(YELLOW)make test-frontend$(NC)                 - Run all frontend tests"
	@echo -e "  $(YELLOW)make test-frontend-unit$(NC)            - Run frontend unit tests (Vitest)"
	@echo -e "  $(YELLOW)make test-frontend-component$(NC)       - Run component tests (Vitest)"
	@echo -e "  $(YELLOW)make test-frontend-smoke$(NC)           - Run smoke tests (Playwright)"
	@echo ""
	@echo -e "$(GREEN)Dev targets:$(NC)"
	@echo -e "  $(YELLOW)make dev$(NC)                           - Run dev server (frontend + backend)"
	@echo -e "  $(YELLOW)make clear-data$(NC)                    - Clear local daemon data files"
	@echo -e "  $(YELLOW)make debug-anime ID=123$(NC)            - Debug why an anime isn't downloading"
	@echo ""
	@echo -e "$(GREEN)Supported platforms:$(NC)"
	@echo -e "  - $(YELLOW)linuxamd64$(NC)  (Linux AMD64)"
	@echo -e "  - $(YELLOW)linuxarm64$(NC)  (Linux ARM64)"
	@echo -e "  - $(YELLOW)windows$(NC)    (Windows AMD64)"
	@echo ""
	@echo -e "$(GREEN)Variables:$(NC)"
	@echo -e "  $(YELLOW)VERSION=$(VERSION)$(NC)      - Version to embed in binaries"
	@echo -e "  $(YELLOW)PLATFORM=$(PLATFORM)$(NC)    - Platform to build (empty = all)"

# Test targets
test:
	@bash scripts/run-all-tests.sh

test-backend: test-backend-unit test-backend-integration

test-backend-unit:
	@test -d src/internal/frontend/dist || (mkdir -p src/internal/frontend/dist && printf '<html></html>' > src/internal/frontend/dist/index.html)
	go test ./src/tests/unit/... ./src/internal/...

test-backend-integration:
	@bash scripts/run-integration-tests.sh

test-frontend: test-frontend-unit test-frontend-component test-frontend-smoke

test-frontend-unit:
	cd src/internal/frontend && bun install --frozen-lockfile && bun run test:unit

test-frontend-component:
	cd src/internal/frontend && bun install --frozen-lockfile && bun run test:component

test-frontend-smoke:
	cd src/internal/frontend && bun install --frozen-lockfile && bun run test:smoke

# Main targets
build:
	@$(foreach p,$(PLATFORMS_TO_PROCESS),$(MAKE) build-$(p);)
	@echo -e "\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "$(GREEN)All builds complete!$(NC)\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"

package:
	@$(foreach p,$(PLATFORMS_TO_PROCESS),$(MAKE) package-$(p);)
	@echo -e "\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "$(GREEN)All packages complete!$(NC)\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "Packages are in: $(GREEN)$(PACKAGES_DIR)/$(NC)\n"

release:
	@$(foreach p,$(PLATFORMS_TO_PROCESS),$(MAKE) build-$(p) package-$(p);)
	@echo -e "\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "$(GREEN)All releases complete!$(NC)\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "Packages are in: $(GREEN)$(PACKAGES_DIR)/$(NC)\n"

# Platform-specific build targets
build-linuxamd64:
	@bash scripts/build.sh linuxamd64 "$(VERSION)"

build-linuxarm64:
	@bash scripts/build.sh linuxarm64 "$(VERSION)"

build-windows:
	@bash scripts/build.sh windows "$(VERSION)"

# Platform-specific release targets
release-linuxamd64: build-linuxamd64 package-linuxamd64
release-linuxarm64: build-linuxarm64 package-linuxarm64
release-windows: build-windows package-windows

# Package targets (require binaries to exist — run make build or make release first)
package-linuxamd64:
	@echo -e "$(YELLOW)Packaging Linux AMD64...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@bash -c ' \
		PACKAGE_DIR=$$(mktemp -d); \
		trap "rm -rf $$PACKAGE_DIR" EXIT INT TERM; \
		PACKAGE_NAME=$(PACKAGE_NAME_linuxamd64); \
		PACKAGES_DIR_ABS="$(CURDIR)/$(PACKAGES_DIR)"; \
		mkdir -p $$PACKAGE_DIR/$$PACKAGE_NAME || exit 1; \
		cp $(BUILD_DIR_linuxamd64)/autoanimedownloader-daemon $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp $(BUILD_DIR_linuxamd64)/autoanimedownloader $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/autoanimedownloader.service $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/Makefile $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/autoanimedownloader.desktop $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp src/internal/tray/icon.png $$PACKAGE_DIR/$$PACKAGE_NAME/icon.png || exit 1; \
		{ \
			echo "# AutoAnimeDownloader - Linux Installation"; \
			echo ""; \
			echo "## Installation"; \
			echo ""; \
			echo "1. Extract this archive"; \
			echo "2. Run \`make install-user\` or \`sudo make install-global\` in the extracted directory"; \
			echo ""; \
			echo "This will install:"; \
			echo "- autoanimedownloader-daemon (with embedded frontend)"; \
			echo "- autoanimedownloader (CLI)"; \
			echo "- systemd user service"; \
			echo ""; \
			echo "### install-user vs install-global"; \
			echo ""; \
			echo "- \`make install-user\` — installs to ~/.local/bin (no sudo required)"; \
			echo "- \`sudo make install-global\` — installs to /usr/local/bin (accessible system-wide)"; \
			echo ""; \
			echo "## Usage"; \
			echo ""; \
			echo "After installation, the daemon will start automatically. You can:"; \
			echo ""; \
			echo "- Access the web UI at http://localhost:8091"; \
			echo "- Use the CLI: \`autoanimedownloader status\`"; \
			echo "- Manage the service: \`systemctl --user status autoanimedownloader\`"; \
			echo ""; \
			echo "## Uninstallation"; \
			echo ""; \
			echo "Run \`make uninstall-user\` or \`sudo make uninstall-global\` in the installation directory."; \
		} > $$PACKAGE_DIR/$$PACKAGE_NAME/README.md || exit 1; \
		cd $$PACKAGE_DIR && \
		zip -r $$PACKAGES_DIR_ABS/$$PACKAGE_NAME.zip $$PACKAGE_NAME > /dev/null || exit 1; \
		cd $$PACKAGES_DIR_ABS && \
		sha256sum $$PACKAGE_NAME.zip > $$PACKAGE_NAME.zip.sha256 || exit 1; \
		printf "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.zip$(NC)\n"'

package-linuxarm64:
	@echo -e "$(YELLOW)Packaging Linux ARM64...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@bash -c ' \
		PACKAGE_DIR=$$(mktemp -d); \
		trap "rm -rf $$PACKAGE_DIR" EXIT INT TERM; \
		PACKAGE_NAME=$(PACKAGE_NAME_linuxarm64); \
		PACKAGES_DIR_ABS="$(CURDIR)/$(PACKAGES_DIR)"; \
		mkdir -p $$PACKAGE_DIR/$$PACKAGE_NAME || exit 1; \
		cp $(BUILD_DIR_linuxarm64)/autoanimedownloader-daemon $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp $(BUILD_DIR_linuxarm64)/autoanimedownloader $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/autoanimedownloader.service $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/Makefile $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp infra/linux/autoanimedownloader.desktop $$PACKAGE_DIR/$$PACKAGE_NAME/ || exit 1; \
		cp src/internal/tray/icon.png $$PACKAGE_DIR/$$PACKAGE_NAME/icon.png || exit 1; \
		{ \
			echo "# AutoAnimeDownloader - Linux ARM64 Installation"; \
			echo ""; \
			echo "## Installation"; \
			echo ""; \
			echo "1. Extract this archive"; \
			echo "2. Run \`make install-user\` or \`sudo make install-global\` in the extracted directory"; \
			echo ""; \
			echo "This will install:"; \
			echo "- autoanimedownloader-daemon (with embedded frontend)"; \
			echo "- autoanimedownloader (CLI)"; \
			echo "- systemd user service"; \
			echo ""; \
			echo "### install-user vs install-global"; \
			echo ""; \
			echo "- \`make install-user\` — installs to ~/.local/bin (no sudo required)"; \
			echo "- \`sudo make install-global\` — installs to /usr/local/bin (accessible system-wide)"; \
			echo ""; \
			echo "## Usage"; \
			echo ""; \
			echo "After installation, the daemon will start automatically. You can:"; \
			echo ""; \
			echo "- Access the web UI at http://localhost:8091"; \
			echo "- Use the CLI: \`autoanimedownloader status\`"; \
			echo "- Manage the service: \`systemctl --user status autoanimedownloader\`"; \
			echo ""; \
			echo "## Uninstallation"; \
			echo ""; \
			echo "Run \`make uninstall-user\` or \`sudo make uninstall-global\` in the installation directory."; \
		} > $$PACKAGE_DIR/$$PACKAGE_NAME/README.md || exit 1; \
		cd $$PACKAGE_DIR && \
		zip -r $$PACKAGES_DIR_ABS/$$PACKAGE_NAME.zip $$PACKAGE_NAME > /dev/null || exit 1; \
		cd $$PACKAGES_DIR_ABS && \
		sha256sum $$PACKAGE_NAME.zip > $$PACKAGE_NAME.zip.sha256 || exit 1; \
		printf "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.zip$(NC)\n"'

package-windows:
	@echo -e "$(YELLOW)Packaging Windows...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@PACKAGE_NAME=$(PACKAGE_NAME_windows); \
	cp $(BUILD_DIR_windows)/autoanimedownloader-daemon.exe $(PACKAGES_DIR)/$$PACKAGE_NAME.exe; \
	cd $(PACKAGES_DIR); \
	sha256sum $$PACKAGE_NAME.exe > $$PACKAGE_NAME.exe.sha256; \
	echo -e "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.exe$(NC)\n"

dev:
	@bash scripts/dev.sh

# Debug why a specific anime isn't downloading. Usage: make debug-anime ID=123
# ID is the AniList MediaList ID (same ID used in /api/v1/animes/{id}/episodes).
debug-anime:
	@go run ./src/cmd/daemon --debug-anime $(ID)

clear-data:
	@bash scripts/clear_data.sh

# Clean build artifacts
clean:
	@echo -e "$(YELLOW)Cleaning build artifacts...$(NC)\n"
	rm -rf build/
	@echo -e "$(GREEN)Clean complete!$(NC)\n"
