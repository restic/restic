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
	pid, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	return pid != unix.Getpgrp(), err
}
