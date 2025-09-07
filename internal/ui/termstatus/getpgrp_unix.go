//go:build unix && !solaris

package termstatus

import "golang.org/x/sys/unix"

func Getpgrp() int { return unix.Getpgrp() }
