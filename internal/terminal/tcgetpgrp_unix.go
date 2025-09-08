//go:build unix && !linux

package terminal

import "golang.org/x/sys/unix"

func tcgetpgrp(ttyfd int) (int, error) {
	return unix.IoctlGetInt(ttyfd, unix.TIOCGPGRP)
}
