//go:build unix && !aix

package terminal

import "golang.org/x/sys/unix"

func tcsetpgrp(fd int, pid int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pid)
}
