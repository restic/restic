package termstatus

import "golang.org/x/sys/unix"

func Getpgrp() int {
	pid, _ := unix.Getpgrp()
	return pid
}
