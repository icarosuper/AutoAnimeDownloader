package diagnostics

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/version"

	"fmt"
	"os"
	"runtime"
	"strings"
)

// host is what the environment section reads from the machine, injectable so the docker/systemd
// branches are testable on any machine.
type host struct {
	goos     string
	getenv   func(string) string
	readFile func(string) ([]byte, error)
}

var realHost = host{goos: runtime.GOOS, getenv: os.Getenv, readFile: os.ReadFile}

// environmentLines is the summary that goes both into the bundle and into the issue URL.
func environmentLines(h host) []string {
	lines := []string{
		"Version: " + version.Version,
		"OS: " + h.goos + "/" + runtime.GOARCH,
	}

	switch h.goos {
	case "windows":
		lines = append(lines, "Windows: "+windowsVersion())
	case "linux":
		lines = append(lines, linuxLines(h)...)
	}
	return lines
}

func linuxLines(h host) []string {
	var lines []string
	// In a container the distro is the image's, not the user's: reporting it would lie.
	if _, err := h.readFile("/.dockerenv"); err == nil {
		lines = append(lines, "Runtime: docker")
	} else {
		lines = append(lines, "Distro: "+distroName(h))
	}

	// A systemd service doesn't inherit the session's variables, so their absence means "not
	// known", not "no desktop".
	if h.getenv("INVOCATION_ID") != "" {
		return append(lines, "Service: systemd", "Desktop: unknown", "Session: unknown")
	}
	return append(lines, "Desktop: "+orNone(h.getenv("XDG_CURRENT_DESKTOP")), "Session: "+orNone(h.getenv("XDG_SESSION_TYPE")))
}

func distroName(h host) string {
	data, err := h.readFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(value, `"'`)
		}
	}
	return "unknown"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// diskLine mirrors what /api/v1/status computes: a full disk silently stops new downloads.
func diskLine(libraryPath string, minFreePercent int) string {
	if libraryPath == "" {
		return "Disk: library not configured"
	}
	total, free, err := files.DiskSpace(libraryPath)
	if err != nil || total == 0 {
		return "Disk: unknown"
	}
	freePercent := float64(free) / float64(total) * 100
	line := fmt.Sprintf("Disk: %.1f GiB free of %.1f GiB (%.0f%%)", gib(free), gib(total), freePercent)
	if minFreePercent > 0 && freePercent < float64(minFreePercent) {
		line += fmt.Sprintf(" - below min_free_disk_percent (%d%%), new downloads are paused", minFreePercent)
	}
	return line
}

func gib(bytes uint64) float64 { return float64(bytes) / (1 << 30) }
