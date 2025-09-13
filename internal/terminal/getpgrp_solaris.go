package terminal

import "golang.org/x/sys/unix"

func getpgrp() int {
	pid, _ := unix.Getpgrp()
	return pid
}
