package processwin

import (
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const stillActive = 259 // windows.STILL_ACTIVE

func Running(pidStr string) bool {
	pidStr = strings.TrimSpace(pidStr)
	if pidStr == "" {
		return false
	}
	pidUint, err := strconv.ParseUint(pidStr, 10, 32)
	if err != nil || pidUint == 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pidUint))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
