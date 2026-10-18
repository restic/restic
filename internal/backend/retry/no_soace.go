//go:build !windows

package retry

import (
	"errors"
	"syscall"
)

var errNoSpace = syscall.ENOSPC

func isNoSpaceError(err error) bool {
	return errors.Is(err, errNoSpace)
}
