//go:build windows

package files

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// DiskSpace returns total and free bytes for the filesystem containing path.
func DiskSpace(path string) (total uint64, free uint64, err error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert path %q: %w", path, err)
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytes, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem for %q: %w", path, err)
	}
	return totalBytes, freeBytes, nil
}
