package diagnostics

import (
	"errors"
	"slices"
	"testing"
)

func fakeHost(files map[string]string, env map[string]string) host {
	return host{
		goos:   "linux",
		getenv: func(k string) string { return env[k] },
		readFile: func(path string) ([]byte, error) {
			if content, ok := files[path]; ok {
				return []byte(content), nil
			}
			return nil, errors.New("not found")
		},
	}
}

func TestEnvironmentReportsDistroAndDesktop(t *testing.T) {
	lines := environmentLines(fakeHost(
		map[string]string{"/etc/os-release": "NAME=Arch\nPRETTY_NAME=\"Arch Linux\"\n"},
		map[string]string{"XDG_CURRENT_DESKTOP": "KDE", "XDG_SESSION_TYPE": "wayland"},
	))
	for _, want := range []string{"Distro: Arch Linux", "Desktop: KDE", "Session: wayland"} {
		if !slices.Contains(lines, want) {
			t.Errorf("missing %q in %v", want, lines)
		}
	}
}

func TestEnvironmentInDockerOmitsTheDistro(t *testing.T) {
	lines := environmentLines(fakeHost(
		map[string]string{"/.dockerenv": "", "/etc/os-release": "PRETTY_NAME=\"Alpine\"\n"}, nil,
	))
	if !slices.Contains(lines, "Runtime: docker") || slices.Contains(lines, "Distro: Alpine") {
		t.Fatalf("got %v", lines)
	}
}

func TestEnvironmentUnderSystemdReportsTheDesktopAsUnknown(t *testing.T) {
	lines := environmentLines(fakeHost(nil, map[string]string{"INVOCATION_ID": "abc"}))
	if !slices.Contains(lines, "Desktop: unknown") || slices.Contains(lines, "Desktop: none") {
		t.Fatalf("got %v", lines)
	}
}
