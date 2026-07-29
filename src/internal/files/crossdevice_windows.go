//go:build windows

package files

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isCrossDevice reports whether err indicates a hardlink attempt across
// different volumes (ERROR_NOT_SAME_DEVICE on Windows).
func isCrossDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}
