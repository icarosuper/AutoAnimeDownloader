// Auto Anime Downloader - Tray Version
// Copyright (C) 2024 AutoAnimeDownloader Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tray

// currentVersion is set via ldflags during build
// Example: go build -ldflags "-X AutoAnimeDownloader/src/internal/tray.currentVersion=1.0.0"
var currentVersion = "dev"

const githubRepo = "icarosuper/AutoAnimeDownloader"

// GetCurrentVersion returns the current version of the application
func GetCurrentVersion() string {
	return currentVersion
}
