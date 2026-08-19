//go:build windows

package statusline

import "syscall"

// syscall only exposes PROCESS_QUERY_INFORMATION; the limited variant needs
// fewer rights and works against processes of other sessions/users.
const processQueryLimitedInformation = 0x1000

// GetExitCodeProcess reports 259 (STILL_ACTIVE) until the process exits.
const stillActive = 259

func processIsAlive(pid uint32) bool {
	if pid == 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
