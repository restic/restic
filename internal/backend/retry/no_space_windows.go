package retry

import (
	"errors"
	"syscall"
)

var errNoSpace = syscall.ERROR_DISK_FULL

func isNoSpaceError(err error) bool {
	return errors.Is(err, errNoSpace) ||
		errors.Is(err, syscall.ERROR_HANDLE_DISK_FULL)
}
