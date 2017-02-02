package xattr

import (
	"syscall"
)

func isNotExist(err error) bool {
	return err == syscall.ENODATA
}
