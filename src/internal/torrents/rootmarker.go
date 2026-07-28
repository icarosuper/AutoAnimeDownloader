package torrents

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// RootMarkerName identifies the download root a session is bound to. It lives INSIDE the
// download folder, so it travels with the folder when the user moves it and disappears
// from the configured path when the folder is moved away or trashed.
const RootMarkerName = ".aad_root"

// rootIDFileName holds the same id OUTSIDE the download folder (next to the resume
// database, in the config folder). The pair is what makes a swap detectable: the expected
// id survives where the user cannot move it, the actual id vanishes with the folder.
const rootIDFileName = "download_root.id"

// newRootID returns a random identifier for a download root.
func newRootID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// readRootID reads an id file, returning "" when it is missing or empty. Any other read
// error is returned: treating an unreadable marker as "missing" would report a swap that
// did not happen and wipe the library records for a permissions glitch.
func readRootID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeRootID(path, id string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(id), 0644)
}
