//go:build !windows

package files

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether err indicates a hardlink/rename attempt across
// different filesystems/volumes (EXDEV on unix).
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
