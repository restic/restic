package termstatus

import (
	"github.com/restic/restic/internal/debug"

	"golang.org/x/sys/unix"
)

// IsProcessBackground reports whether the current process is running in the
// background. fd must be a file descriptor for the terminal.
func IsProcessBackground(fd uintptr) bool {
	bg, err := isProcessBackground(fd)
	if err != nil {
		debug.Log("Can't check if we are in the background. Using default behaviour. Error: %s\n", err.Error())
		return false
	}
	return bg
}

func isProcessBackground(fd uintptr) (bool, error) {
	// We need to use IoctlGetUint32 here, because pid_t is 32-bit even on
	// 64-bit Linux. IoctlGetInt doesn't work on big-endian platforms:
	// https://github.com/golang/go/issues/45585
	// https://github.com/golang/go/issues/60429
	pid, err := unix.IoctlGetUint32(int(fd), unix.TIOCGPGRP)
	return int(pid) != unix.Getpgrp(), err
}
