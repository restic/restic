//go:build unix && !aix

package terminal

import "golang.org/x/sys/unix"

func Tcsetpgrp(fd int, pid int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pid)
}
