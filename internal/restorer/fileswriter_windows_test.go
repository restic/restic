package restorer

import "syscall"

func notEmptyDirError() error {
	return syscall.ERROR_DIR_NOT_EMPTY
}
