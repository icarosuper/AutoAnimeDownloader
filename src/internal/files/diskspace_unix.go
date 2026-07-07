//go:build linux || darwin

package files

import (
	"fmt"
	"syscall"
)

// DiskSpace returns total and free bytes for the filesystem containing path.
func DiskSpace(path string) (total uint64, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem for %q: %w", path, err)
	}
	total = uint64(stat.Blocks) * uint64(stat.Bsize)
	free = uint64(stat.Bavail) * uint64(stat.Bsize)
	return total, free, nil
}
