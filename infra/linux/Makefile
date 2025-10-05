# If PREFIX isn't provided, we check for $(DESTDIR)/usr/local and use that if it exists.
# Otherwise we fall back to using /usr.
LOCAL != test -d $(DESTDIR)/usr/local && echo -n "/local" || echo -n ""
LOCAL ?= $(shell test -d $(DESTDIR)/usr/local && echo "/local" || echo "")
PREFIX ?= /usr$(LOCAL)
AppID := "io.github.icarosuper.autoanimedownloader"
Exec := "AutoAnimeDownloader"
Icon := "io.github.icarosuper.autoanimedownloader.png"
ServiceName := "autoanimedownloader.service"

default:
	@echo "Run 'make install' to install app"
	@echo "Run 'make uninstall' to uninstall app"

install: install-app install-service
	@echo ""
	@echo "Installation complete!"
	@echo "Application installed in ~/.local/bin/$(Exec)"
	@echo "Systemd service installed and enabled."

install-app:
	install -Dm00644 usr/local/share/applications/$(AppID).desktop $(DESTDIR)$(HOME)/.local/share/applications/$(AppID).desktop
	install -Dm00755 usr/local/bin/$(Exec) $(DESTDIR)$(HOME)/.local/bin/$(Exec)
	install -Dm00644 usr/local/share/pixmaps/$(Icon) $(DESTDIR)$(HOME)/.local/share/icons/$(Icon)
	sed -i -e "s,Exec=$(Exec),Exec=$(DESTDIR)$(HOME)/.local/bin/$(Exec),g" $(DESTDIR)$(HOME)/.local/share/applications/$(AppID).desktop

uninstall: uninstall-service uninstall-app
	@echo "Uninstallation complete!"

uninstall-app:
	-rm $(DESTDIR)$(HOME)/.local/share/applications/$(AppID).desktop
	-rm $(DESTDIR)$(HOME)/.local/bin/$(Exec)
	-rm $(DESTDIR)$(HOME)/.local/share/icons/$(Icon)

install-service:
	@echo "Installing systemd user service..."
	@mkdir -p $(HOME)/.config/systemd/user
	install -Dm00644 $(ServiceName) $(HOME)/.config/systemd/user/$(ServiceName)
	@systemctl --user daemon-reload
	@systemctl --user enable $(ServiceName)
	@systemctl --user start $(ServiceName)

uninstall-service:
	@echo "Stopping and disabling service..."
	-@systemctl --user stop $(ServiceName) 2>/dev/null
	-@systemctl --user disable $(ServiceName) 2>/dev/null
	-@rm $(HOME)/.config/systemd/user/$(ServiceName)
	@systemctl --user daemon-reload
