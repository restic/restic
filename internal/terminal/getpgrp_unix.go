//go:build unix && !solaris

package terminal

import "golang.org/x/sys/unix"

func getpgrp() int { return unix.Getpgrp() }
