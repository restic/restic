//go:build unix && !solaris

package terminal

import "golang.org/x/sys/unix"

func Getpgrp() int { return unix.Getpgrp() }
