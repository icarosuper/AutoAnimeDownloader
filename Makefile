# Makefile for AutoAnimeDownloader
# Cross-platform builds using Docker
# Works on Linux, macOS, and WSL (Windows Subsystem for Linux)

.PHONY: build package release clean help check-docker
.PHONY: build-linuxamd64 build-linuxarm64 build-windows
.PHONY: package-linuxamd64 package-linuxarm64 package-windows
.PHONY: release-linuxamd64 release-linuxarm64 release-windows

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[1;33m
RED := \033[0;31m
NC := \033[0m # No Color

# Get version from git tag or use default
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev")

# Platform support
PLATFORMS := linuxamd64 linuxarm64 windows
PLATFORM ?=

# Platform mappings
DOCKERFILE_linuxamd64 := Dockerfile.build.amd64
DOCKERFILE_linuxarm64 := Dockerfile.build.arm64
DOCKERFILE_windows := Dockerfile.build.windows

BUILD_DIR_linuxamd64 := build/linux-amd64
BUILD_DIR_linuxarm64 := build/linux-arm64
BUILD_DIR_windows := build/windows-amd64

DOCKER_IMAGE_linuxamd64 := aad-build-amd64
DOCKER_IMAGE_linuxarm64 := aad-build-arm64
DOCKER_IMAGE_windows := aad-build-windows

CONTAINER_linuxamd64 := aad-temp-amd64
CONTAINER_linuxarm64 := aad-temp-arm64
CONTAINER_windows := aad-temp-windows

PACKAGE_NAME_linuxamd64 := AutoAnimeDownloader_Linux_x86
PACKAGE_NAME_linuxarm64 := AutoAnimeDownloader_Linux_Arm64
PACKAGE_NAME_windows := AutoAnimeDownloader_Windows

PACKAGES_DIR := build/packages

# Helper function to get platforms to process
ifeq ($(PLATFORM),)
  PLATFORMS_TO_PROCESS := $(PLATFORMS)
else
  PLATFORMS_TO_PROCESS := $(PLATFORM)
endif

help:
	@echo -e "$(GREEN)Available targets:$(NC)"
	@echo -e "  $(YELLOW)make build$(NC)                    - Build for all platforms"
	@echo -e "  $(YELLOW)make build PLATFORM=windows$(NC) - Build for specific platform"
	@echo -e "  $(YELLOW)make package$(NC)                 - Package for all platforms"
	@echo -e "  $(YELLOW)make package PLATFORM=linuxamd64$(NC) - Package for specific platform"
	@echo -e "  $(YELLOW)make release$(NC)                 - Build + package for all platforms"
	@echo -e "  $(YELLOW)make release PLATFORM=linuxarm64$(NC) - Build + package for specific platform"
	@echo -e "  $(YELLOW)make clean$(NC)                  - Remove build artifacts"
	@echo ""
	@echo -e "$(GREEN)Supported platforms:$(NC)"
	@echo -e "  - $(YELLOW)linuxamd64$(NC)  (Linux AMD64)"
	@echo -e "  - $(YELLOW)linuxarm64$(NC)  (Linux ARM64)"
	@echo -e "  - $(YELLOW)windows$(NC)    (Windows AMD64)"
	@echo ""
	@echo -e "$(GREEN)Variables:$(NC)"
	@echo -e "  $(YELLOW)VERSION=$(VERSION)$(NC)      - Version to embed in binaries"
	@echo -e "  $(YELLOW)PLATFORM=$(PLATFORM)$(NC)    - Platform to build (empty = all)"

# Main targets
build: check-docker
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

release: check-docker
	@$(foreach p,$(PLATFORMS_TO_PROCESS),$(MAKE) build-$(p) && $(MAKE) package-$(p);)
	@echo -e "\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "$(GREEN)All releases complete!$(NC)\n"
	@echo -e "$(GREEN)════════════════════════════════════════$(NC)\n"
	@echo -e "Packages are in: $(GREEN)$(PACKAGES_DIR)/$(NC)\n"

# Platform-specific build targets
build-linuxamd64: $(BUILD_DIR_linuxamd64)/autoanimedownloader-daemon $(BUILD_DIR_linuxamd64)/autoanimedownloader
	@echo -e "$(GREEN)✓ Linux AMD64 build complete!$(NC)\n"

build-linuxarm64: $(BUILD_DIR_linuxarm64)/autoanimedownloader-daemon $(BUILD_DIR_linuxarm64)/autoanimedownloader
	@echo -e "$(GREEN)✓ Linux ARM64 build complete!$(NC)\n"

build-windows: $(BUILD_DIR_windows)/autoanimedownloader-daemon.exe $(BUILD_DIR_windows)/autoanimedownloader.exe
	@echo -e "$(GREEN)✓ Windows AMD64 build complete!$(NC)\n"

# Build binaries using Docker
$(BUILD_DIR_linuxamd64)/autoanimedownloader-daemon $(BUILD_DIR_linuxamd64)/autoanimedownloader: PLATFORM := linuxamd64
$(BUILD_DIR_linuxamd64)/autoanimedownloader-daemon $(BUILD_DIR_linuxamd64)/autoanimedownloader: | $(BUILD_DIR_linuxamd64)
	@echo -e "$(YELLOW)Building for Linux AMD64...$(NC)\n"
	@$(MAKE) docker-build PLATFORM=linuxamd64

$(BUILD_DIR_linuxarm64)/autoanimedownloader-daemon $(BUILD_DIR_linuxarm64)/autoanimedownloader: PLATFORM := linuxarm64
$(BUILD_DIR_linuxarm64)/autoanimedownloader-daemon $(BUILD_DIR_linuxarm64)/autoanimedownloader: | $(BUILD_DIR_linuxarm64)
	@echo -e "$(YELLOW)Building for Linux ARM64...$(NC)\n"
	@$(MAKE) docker-build PLATFORM=linuxarm64

$(BUILD_DIR_windows)/autoanimedownloader-daemon.exe $(BUILD_DIR_windows)/autoanimedownloader.exe: PLATFORM := windows
$(BUILD_DIR_windows)/autoanimedownloader-daemon.exe $(BUILD_DIR_windows)/autoanimedownloader.exe: | $(BUILD_DIR_windows)
	@echo -e "$(YELLOW)Building for Windows AMD64...$(NC)\n"
	@$(MAKE) docker-build PLATFORM=windows

# Generic Docker build target
# ARM64 always uses buildx (cross-compilation), others use classic docker build locally (faster)
# CI/CD always uses buildx
docker-build: check-docker
	@PLATFORM=$(PLATFORM); \
	if [ -n "$$CI" ] || [ -n "$$GITHUB_ACTIONS" ]; then \
		if command -v docker >/dev/null 2>&1 && docker buildx version >/dev/null 2>&1; then \
			echo -e "$(YELLOW)Using docker buildx (CI/CD)...$(NC)\n"; \
			$(MAKE) docker-buildx-build PLATFORM=$$PLATFORM; \
		else \
			echo -e "$(YELLOW)Using docker build...$(NC)\n"; \
			$(MAKE) docker-build-classic PLATFORM=$$PLATFORM; \
		fi; \
	elif [ "$$PLATFORM" = "linuxarm64" ]; then \
		if command -v docker >/dev/null 2>&1 && docker buildx version >/dev/null 2>&1; then \
			echo -e "$(YELLOW)Using docker buildx (required for ARM64 cross-compilation)...$(NC)\n"; \
			$(MAKE) docker-buildx-build PLATFORM=$$PLATFORM; \
		else \
			echo -e "$(RED)Error: docker buildx is required for ARM64 builds$(NC)\n"; \
			echo -e "$(YELLOW)Please install docker buildx: docker buildx install$(NC)\n"; \
			exit 1; \
		fi; \
	else \
		echo -e "$(YELLOW)Using docker build (faster for local builds)...$(NC)\n"; \
		$(MAKE) docker-build-classic PLATFORM=$$PLATFORM; \
	fi

# Docker buildx build (preferred, used in CI/CD)
docker-buildx-build:
	@PLATFORM=$(PLATFORM); \
	case "$$PLATFORM" in \
		linuxamd64) \
			DOCKERFILE=$(DOCKERFILE_linuxamd64); \
			BUILD_DIR=$(BUILD_DIR_linuxamd64); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_linuxamd64); \
			CONTAINER=$(CONTAINER_linuxamd64); \
			;; \
		linuxarm64) \
			DOCKERFILE=$(DOCKERFILE_linuxarm64); \
			BUILD_DIR=$(BUILD_DIR_linuxarm64); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_linuxarm64); \
			CONTAINER=$(CONTAINER_linuxarm64); \
			;; \
		windows) \
			DOCKERFILE=$(DOCKERFILE_windows); \
			BUILD_DIR=$(BUILD_DIR_windows); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_windows); \
			CONTAINER=$(CONTAINER_windows); \
			;; \
	esac; \
	echo -e "$(YELLOW)Building Docker image with buildx...$(NC)\n"; \
	if [ "$$PLATFORM" = "linuxarm64" ]; then \
		docker buildx build \
			--platform linux/arm64 \
			--load \
			-f $$DOCKERFILE \
			--build-arg VERSION=$(VERSION) \
			-t $$DOCKER_IMAGE \
			. || exit 1; \
	else \
		docker buildx build \
			--load \
			-f $$DOCKERFILE \
			--build-arg VERSION=$(VERSION) \
			-t $$DOCKER_IMAGE \
			. || exit 1; \
	fi; \
	echo -e "$(YELLOW)Extracting binaries...$(NC)\n"; \
	docker rm -f $$CONTAINER 2>/dev/null || true; \
	docker create --name $$CONTAINER $$DOCKER_IMAGE; \
	docker cp $$CONTAINER:/output/. $$BUILD_DIR/; \
	docker rm $$CONTAINER; \
	$(MAKE) rename-binaries PLATFORM=$$PLATFORM; \
	$(MAKE) checksums PLATFORM=$$PLATFORM

# Classic Docker build (fallback for local development)
docker-build-classic:
	@PLATFORM=$(PLATFORM); \
	case "$$PLATFORM" in \
		linuxamd64) \
			DOCKERFILE=$(DOCKERFILE_linuxamd64); \
			BUILD_DIR=$(BUILD_DIR_linuxamd64); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_linuxamd64); \
			CONTAINER=$(CONTAINER_linuxamd64); \
			;; \
		linuxarm64) \
			DOCKERFILE=$(DOCKERFILE_linuxarm64); \
			BUILD_DIR=$(BUILD_DIR_linuxarm64); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_linuxarm64); \
			CONTAINER=$(CONTAINER_linuxarm64); \
			;; \
		windows) \
			DOCKERFILE=$(DOCKERFILE_windows); \
			BUILD_DIR=$(BUILD_DIR_windows); \
			DOCKER_IMAGE=$(DOCKER_IMAGE_windows); \
			CONTAINER=$(CONTAINER_windows); \
			;; \
	esac; \
	echo -e "$(YELLOW)Building Docker image...$(NC)\n"; \
	docker build -f $$DOCKERFILE --build-arg VERSION=$(VERSION) -t $$DOCKER_IMAGE .; \
	echo -e "$(YELLOW)Extracting binaries...$(NC)\n"; \
	docker rm -f $$CONTAINER 2>/dev/null || true; \
	docker create --name $$CONTAINER $$DOCKER_IMAGE; \
	docker cp $$CONTAINER:/output/. $$BUILD_DIR/; \
	docker rm $$CONTAINER; \
	$(MAKE) rename-binaries PLATFORM=$$PLATFORM; \
	$(MAKE) checksums PLATFORM=$$PLATFORM

# Rename binaries to expected names (no longer needed, Dockerfiles generate correct names)
rename-binaries:
	@# Binaries are already named correctly by Dockerfiles

# Generate checksums
checksums:
	@PLATFORM=$(PLATFORM); \
	case "$$PLATFORM" in \
		linuxamd64) BUILD_DIR=$(BUILD_DIR_linuxamd64) ;; \
		linuxarm64) BUILD_DIR=$(BUILD_DIR_linuxarm64) ;; \
		windows) BUILD_DIR=$(BUILD_DIR_windows) ;; \
	esac; \
	cd $$BUILD_DIR && \
	if [ "$$PLATFORM" = "windows" ]; then \
		sha256sum autoanimedownloader-daemon.exe > autoanimedownloader-daemon.exe.sha256 2>/dev/null || true; \
		sha256sum autoanimedownloader.exe > autoanimedownloader.exe.sha256 2>/dev/null || true; \
	else \
		sha256sum autoanimedownloader-daemon > autoanimedownloader-daemon.sha256 2>/dev/null || true; \
		sha256sum autoanimedownloader > autoanimedownloader.sha256 2>/dev/null || true; \
	fi

# Create build directories
$(BUILD_DIR_linuxamd64) $(BUILD_DIR_linuxarm64) $(BUILD_DIR_windows):
	@mkdir -p $@

# Package targets
package-linuxamd64: $(BUILD_DIR_linuxamd64)/autoanimedownloader-daemon
	@echo -e "$(YELLOW)Packaging Linux AMD64...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@bash -c ' \
		PACKAGE_DIR=$$(mktemp -d); \
		trap "rm -rf $$PACKAGE_DIR" EXIT INT TERM; \
		PACKAGE_NAME=$(PACKAGE_NAME_linuxamd64); \
		PACKAGES_DIR_ABS="$(shell pwd)/$(PACKAGES_DIR)"; \
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
			echo "2. Run \`make install\` in the extracted directory"; \
			echo ""; \
			echo "This will install:"; \
			echo "- autoanimedownloader-daemon (with embedded frontend)"; \
			echo "- autoanimedownloader (CLI)"; \
			echo "- systemd user service"; \
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
			echo "Run \`make uninstall\` in the installation directory."; \
		} > $$PACKAGE_DIR/$$PACKAGE_NAME/README.md || exit 1; \
		cd $$PACKAGE_DIR && \
		zip -r $$PACKAGES_DIR_ABS/$$PACKAGE_NAME.zip $$PACKAGE_NAME > /dev/null || exit 1; \
		cd $$PACKAGES_DIR_ABS && \
		sha256sum $$PACKAGE_NAME.zip > $$PACKAGE_NAME.zip.sha256 || exit 1; \
		printf "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.zip$(NC)\n"'

package-linuxarm64: $(BUILD_DIR_linuxarm64)/autoanimedownloader-daemon
	@echo -e "$(YELLOW)Packaging Linux ARM64...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@bash -c ' \
		PACKAGE_DIR=$$(mktemp -d); \
		trap "rm -rf $$PACKAGE_DIR" EXIT INT TERM; \
		PACKAGE_NAME=$(PACKAGE_NAME_linuxarm64); \
		PACKAGES_DIR_ABS="$(shell pwd)/$(PACKAGES_DIR)"; \
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
			echo "2. Run \`make install\` in the extracted directory"; \
			echo ""; \
			echo "This will install:"; \
			echo "- autoanimedownloader-daemon (with embedded frontend)"; \
			echo "- autoanimedownloader (CLI)"; \
			echo "- systemd user service"; \
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
			echo "Run \`make uninstall\` in the installation directory."; \
		} > $$PACKAGE_DIR/$$PACKAGE_NAME/README.md || exit 1; \
		cd $$PACKAGE_DIR && \
		zip -r $$PACKAGES_DIR_ABS/$$PACKAGE_NAME.zip $$PACKAGE_NAME > /dev/null || exit 1; \
		cd $$PACKAGES_DIR_ABS && \
		sha256sum $$PACKAGE_NAME.zip > $$PACKAGE_NAME.zip.sha256 || exit 1; \
		printf "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.zip$(NC)\n"'

package-windows: $(BUILD_DIR_windows)/autoanimedownloader-daemon.exe
	@echo -e "$(YELLOW)Packaging Windows...$(NC)\n"
	@mkdir -p $(PACKAGES_DIR)
	@PACKAGE_NAME=$(PACKAGE_NAME_windows); \
	cp $(BUILD_DIR_windows)/autoanimedownloader-daemon.exe $(PACKAGES_DIR)/$$PACKAGE_NAME.exe; \
	cd $(PACKAGES_DIR); \
	sha256sum $$PACKAGE_NAME.exe > $$PACKAGE_NAME.exe.sha256; \
	echo -e "$(GREEN)Created: $(PACKAGES_DIR)/$$PACKAGE_NAME.exe$(NC)\n"

# Check Docker availability
check-docker:
	@command -v docker >/dev/null 2>&1 || { \
		echo -e "$(RED)Error: Docker is not installed or not in PATH$(NC)\n"; \
		echo -e "$(YELLOW)Please install Docker to build for all platforms$(NC)\n"; \
		exit 1; \
	}

# Clean build artifacts
clean:
	@echo -e "$(YELLOW)Cleaning build artifacts...$(NC)\n"
	rm -rf build/
	@echo -e "$(GREEN)Clean complete!$(NC)\n"

# Legacy targets (for backward compatibility)
build-linux-amd64: build-linuxamd64
build-linux-arm64: build-linuxarm64
build-all: build
