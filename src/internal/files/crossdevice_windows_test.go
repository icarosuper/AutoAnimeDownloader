//go:build windows

package files

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIsCrossDeviceWindows(t *testing.T) {
	if !isCrossDevice(windows.ERROR_NOT_SAME_DEVICE) {
		t.Errorf("isCrossDevice(ERROR_NOT_SAME_DEVICE) = false, want true")
	}
	if isCrossDevice(errors.New("some other error")) {
		t.Errorf("isCrossDevice(other) = true, want false")
	}
	if isCrossDevice(windows.ERROR_FILE_NOT_FOUND) {
		t.Errorf("isCrossDevice(ERROR_FILE_NOT_FOUND) = true, want false")
	}
}
