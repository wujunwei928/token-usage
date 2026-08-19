//go:build !windows

package statusline

import "syscall"

func processIsAlive(pid uint32) bool {
	return pid != 0 && syscall.Kill(int(pid), 0) == nil
}
