//go:build unix

package termstatus

import "github.com/restic/restic/internal/debug"

// IsProcessBackground reports whether the current process is running in the
// background. fd must be a file descriptor for the terminal.
func IsProcessBackground(fd uintptr) bool {
	bg, err := isProcessBackground(int(fd))
	if err != nil {
		debug.Log("Can't check if we are in the background. Using default behaviour. Error: %s\n", err.Error())
		return false
	}
	return bg
}

func isProcessBackground(fd int) (bg bool, err error) {
	pgid, err := Tcgetpgrp(fd)
	if err != nil {
		return false, err
	}
	return pgid != Getpgrp(), nil
}
