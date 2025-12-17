# Makefile for AutoAnimeDownloader installation
# Run 'make install' to install daemon, CLI and systemd service

DAEMON_BINARY := AutoAnimeDownloader-daemon
CLI_BINARY := AutoAnimeDownloader-cli
SERVICE_NAME := autoanimedownloader.service
INSTALL_DIR := $(HOME)/.local/bin
SERVICE_DIR := $(HOME)/.config/systemd/user

default:
	@echo "Run 'make install' to install AutoAnimeDownloader"
	@echo "Run 'make uninstall' to uninstall AutoAnimeDownloader"

install: install-binaries install-service
	@echo ""
	@echo "Installation complete!"
	@echo "Daemon installed in $(INSTALL_DIR)/$(DAEMON_BINARY)"
	@echo "CLI installed in $(INSTALL_DIR)/$(CLI_BINARY)"
	@echo "Systemd service installed and enabled."

install-binaries:
	@echo "Installing binaries..."
	@mkdir -p $(INSTALL_DIR)
	install -Dm00755 $(DAEMON_BINARY) $(INSTALL_DIR)/$(DAEMON_BINARY)
	install -Dm00755 $(CLI_BINARY) $(INSTALL_DIR)/$(CLI_BINARY)
	@echo "Binaries installed successfully."

install-service:
	@echo "Installing systemd user service..."
	@mkdir -p $(SERVICE_DIR)
	install -Dm00644 $(SERVICE_NAME) $(SERVICE_DIR)/$(SERVICE_NAME)
	@systemctl --user daemon-reload
	@echo "Stopping service if already running..."
	-@systemctl --user stop $(SERVICE_NAME) 2>/dev/null || true
	@systemctl --user enable $(SERVICE_NAME)
	@systemctl --user start $(SERVICE_NAME)
	@echo "Service installed and started."

uninstall: uninstall-service uninstall-binaries
	@echo "Uninstallation complete!"

uninstall-binaries:
	@echo "Removing binaries..."
	-rm -f $(INSTALL_DIR)/$(DAEMON_BINARY)
	-rm -f $(INSTALL_DIR)/$(CLI_BINARY)
	@echo "Binaries removed."

uninstall-service:
	@echo "Stopping and disabling service..."
	-@systemctl --user stop $(SERVICE_NAME) 2>/dev/null || true
	-@systemctl --user disable $(SERVICE_NAME) 2>/dev/null || true
	-@rm -f $(SERVICE_DIR)/$(SERVICE_NAME)
	@systemctl --user daemon-reload
	@echo "Service removed."
