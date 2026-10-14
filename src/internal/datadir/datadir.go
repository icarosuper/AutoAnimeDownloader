// Package datadir answers where the app keeps its config and state. It's a leaf on purpose (it
// imports nothing from the project): logger needs it, and files imports logger, so the function
// can't live in files.
package datadir

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const folderName = ".autoAnimeDownloader"

// Path returns the data dir, creating it when missing: %APPDATA%\.autoAnimeDownloader on Windows,
// $HOME/.autoAnimeDownloader elsewhere. Not cached: tests point HOME at a temp dir with t.Setenv.
func Path() (string, error) {
	base := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	dir := filepath.Join(base, folderName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}
	return dir, nil
}
