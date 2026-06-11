# Makefile for AutoAnimeDownloader installation
# Run 'make install-user' or 'sudo make install-global'

DAEMON_BINARY := autoanimedownloader-daemon
CLI_BINARY := autoanimedownloader
SERVICE_NAME := autoanimedownloader.service
DESKTOP_FILE := autoanimedownloader.desktop
ICON_FILE := icon.png
USER_INSTALL_DIR := $(HOME)/.local/bin
GLOBAL_INSTALL_DIR := /usr/local/bin
SERVICE_DIR := $(HOME)/.config/systemd/user
APPLICATIONS_DIR := $(HOME)/.local/share/applications
ICONS_DIR := $(HOME)/.local/share/icons

# When running as root via sudo, user-level ops (systemctl --user, desktop entries)
# must run as the real user. These macros detect the case and delegate accordingly.

define do-install-binaries
	@echo "Installing binaries..."
	@mkdir -p $(1)
	install -Dm00755 $(DAEMON_BINARY) $(1)/$(DAEMON_BINARY)
	install -Dm00755 $(CLI_BINARY) $(1)/$(CLI_BINARY)
	@echo "Binaries installed successfully."
endef

define do-install-service
	@echo "Installing systemd user service..."
	@if [ "$$(id -u)" = "0" ] && [ -n "$$SUDO_USER" ]; then \
		_UID=$$(id -u "$$SUDO_USER"); \
		_HOME=$$(getent passwd "$$SUDO_USER" | cut -d: -f6); \
		mkdir -p "$$_HOME/.config/systemd/user"; \
		sed 's|%h/.local/bin|$(1)|g' $(SERVICE_NAME) | sudo -u "$$SUDO_USER" tee "$$_HOME/.config/systemd/user/$(SERVICE_NAME)" > /dev/null; \
		sudo -u "$$SUDO_USER" env HOME="$$_HOME" XDG_RUNTIME_DIR=/run/user/$$_UID DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$$_UID/bus \
			bash -c 'systemctl --user daemon-reload && systemctl --user stop $(SERVICE_NAME) 2>/dev/null; systemctl --user enable $(SERVICE_NAME) && systemctl --user start $(SERVICE_NAME)'; \
	else \
		mkdir -p "$(SERVICE_DIR)"; \
		sed 's|%h/.local/bin|$(1)|g' $(SERVICE_NAME) > "$(SERVICE_DIR)/$(SERVICE_NAME)"; \
		chmod 644 "$(SERVICE_DIR)/$(SERVICE_NAME)"; \
		systemctl --user daemon-reload; \
		systemctl --user stop $(SERVICE_NAME) 2>/dev/null || true; \
		systemctl --user enable $(SERVICE_NAME); \
		systemctl --user start $(SERVICE_NAME); \
	fi
	@echo "Service installed and started."
endef

define do-install-desktop
	@echo "Installing desktop entry..."
	@if [ "$$(id -u)" = "0" ] && [ -n "$$SUDO_USER" ]; then \
		_HOME=$$(getent passwd "$$SUDO_USER" | cut -d: -f6); \
		sudo -u "$$SUDO_USER" install -Dm00644 $(DESKTOP_FILE) "$$_HOME/.local/share/applications/$(DESKTOP_FILE)"; \
		sudo -u "$$SUDO_USER" install -Dm00644 $(ICON_FILE) "$$_HOME/.local/share/icons/autoanimedownloader.png"; \
	else \
		install -Dm00644 $(DESKTOP_FILE) "$(APPLICATIONS_DIR)/$(DESKTOP_FILE)"; \
		install -Dm00644 $(ICON_FILE) "$(ICONS_DIR)/autoanimedownloader.png"; \
	fi
	@echo "Desktop entry installed successfully."
endef

define do-uninstall-binaries
	@echo "Removing binaries..."
	-rm -f $(1)/$(DAEMON_BINARY)
	-rm -f $(1)/$(CLI_BINARY)
	@echo "Binaries removed."
endef

define do-uninstall-service
	@echo "Stopping and disabling service..."
	@if [ "$$(id -u)" = "0" ] && [ -n "$$SUDO_USER" ]; then \
		_UID=$$(id -u "$$SUDO_USER"); \
		_HOME=$$(getent passwd "$$SUDO_USER" | cut -d: -f6); \
		sudo -u "$$SUDO_USER" env HOME="$$_HOME" XDG_RUNTIME_DIR=/run/user/$$_UID DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$$_UID/bus \
			bash -c 'systemctl --user stop $(SERVICE_NAME) 2>/dev/null; systemctl --user disable $(SERVICE_NAME) 2>/dev/null'; \
		sudo -u "$$SUDO_USER" rm -f "$$_HOME/.config/systemd/user/$(SERVICE_NAME)"; \
		sudo -u "$$SUDO_USER" env HOME="$$_HOME" XDG_RUNTIME_DIR=/run/user/$$_UID DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$$_UID/bus \
			systemctl --user daemon-reload; \
	else \
		systemctl --user stop $(SERVICE_NAME) 2>/dev/null || true; \
		systemctl --user disable $(SERVICE_NAME) 2>/dev/null || true; \
		rm -f "$(SERVICE_DIR)/$(SERVICE_NAME)"; \
		systemctl --user daemon-reload; \
	fi
	@echo "Service removed."
endef

define do-uninstall-desktop
	@echo "Removing desktop entry..."
	@if [ "$$(id -u)" = "0" ] && [ -n "$$SUDO_USER" ]; then \
		_HOME=$$(getent passwd "$$SUDO_USER" | cut -d: -f6); \
		sudo -u "$$SUDO_USER" rm -f "$$_HOME/.local/share/applications/$(DESKTOP_FILE)"; \
		sudo -u "$$SUDO_USER" rm -f "$$_HOME/.local/share/icons/autoanimedownloader.png"; \
	else \
		rm -f "$(APPLICATIONS_DIR)/$(DESKTOP_FILE)"; \
		rm -f "$(ICONS_DIR)/autoanimedownloader.png"; \
	fi
	@echo "Desktop entry removed."
endef

.PHONY: install-user install-global uninstall-user uninstall-global

default:
	@echo "Run 'make install-user' to install for current user (~/.local/bin)"
	@echo "Run 'sudo make install-global' to install system-wide (/usr/local/bin)"
	@echo "Run 'make uninstall-user' or 'sudo make uninstall-global' to uninstall"

install-user:
	$(call do-install-binaries,$(USER_INSTALL_DIR))
	$(call do-install-service,$(USER_INSTALL_DIR))
	$(call do-install-desktop)
	@echo ""
	@echo "Installation complete!"
	@echo "Daemon installed in $(USER_INSTALL_DIR)/$(DAEMON_BINARY)"
	@echo "CLI installed in $(USER_INSTALL_DIR)/$(CLI_BINARY)"
	@echo "Systemd service installed and enabled."
	@echo "Desktop entry installed."

install-global:
	@if [ "$$(id -u)" != "0" ]; then echo "Error: sudo required. Run: sudo make install-global"; exit 1; fi
	$(call do-install-binaries,$(GLOBAL_INSTALL_DIR))
	$(call do-install-service,$(GLOBAL_INSTALL_DIR))
	$(call do-install-desktop)
	@echo ""
	@echo "Installation complete!"
	@echo "Daemon installed in $(GLOBAL_INSTALL_DIR)/$(DAEMON_BINARY)"
	@echo "CLI installed in $(GLOBAL_INSTALL_DIR)/$(CLI_BINARY)"
	@echo "Systemd service installed and enabled."
	@echo "Desktop entry installed."

uninstall-user:
	$(call do-uninstall-service)
	$(call do-uninstall-binaries,$(USER_INSTALL_DIR))
	$(call do-uninstall-desktop)
	@echo "Uninstallation complete!"

uninstall-global:
	@if [ "$$(id -u)" != "0" ]; then echo "Error: sudo required. Run: sudo make uninstall-global"; exit 1; fi
	$(call do-uninstall-service)
	$(call do-uninstall-binaries,$(GLOBAL_INSTALL_DIR))
	$(call do-uninstall-desktop)
	@echo "Uninstallation complete!"
