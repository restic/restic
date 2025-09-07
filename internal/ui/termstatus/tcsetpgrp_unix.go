//go:build unix && !aix

package termstatus

import "golang.org/x/sys/unix"

func Tcsetpgrp(fd int, pid int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pid)
}
