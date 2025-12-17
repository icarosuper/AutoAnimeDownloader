//go:build windows

package cli

import "syscall"

// getSysProcAttr returns nil on Windows since Setpgid is not available
// This function is only compiled on Windows to provide a stub
func getSysProcAttr() *syscall.SysProcAttr {
	return nil
}

