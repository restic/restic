package termstatus

import "golang.org/x/sys/unix"

func Tcgetpgrp(ttyfd int) (int, error) {
	// We need to use IoctlGetUint32 here, because pid_t is 32-bit even on
	// 64-bit Linux. IoctlGetInt doesn't work on big-endian platforms:
	// https://github.com/golang/go/issues/45585
	// https://github.com/golang/go/issues/60429
	pid, err := unix.IoctlGetUint32(ttyfd, unix.TIOCGPGRP)
	return int(pid), err
}
