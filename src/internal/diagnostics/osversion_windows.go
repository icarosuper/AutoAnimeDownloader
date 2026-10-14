package diagnostics

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func windowsVersion() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
