package termstatus

import "golang.org/x/sys/unix"

func Tcsetpgrp(fd int, pid int) error {
	// The second argument to IoctlSetPointerInt has type int on AIX,
	// but the constant overflows 64-bit int, hence the two-step cast.
	req := uint(unix.TIOCSPGRP)
	return unix.IoctlSetPointerInt(fd, int(req), pid)
}
