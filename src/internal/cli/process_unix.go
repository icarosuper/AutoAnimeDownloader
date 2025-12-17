//go:build !windows

package cli

import "syscall"

// getSysProcAttr returns SysProcAttr for Unix systems
// This function is only compiled on Unix systems to avoid Windows compilation errors
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

