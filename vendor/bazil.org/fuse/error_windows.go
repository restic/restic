package fuse

import (
	"syscall"
)

const (
	ENODATA = Errno(syscall.ENODATA)
)

const (
	errNoXattr = ENODATA
)
