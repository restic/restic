//go:build unix && !linux

package termstatus

import "golang.org/x/sys/unix"

func Tcgetpgrp(ttyfd int) (int, error) {
	return unix.IoctlGetInt(ttyfd, unix.TIOCGPGRP)
}
