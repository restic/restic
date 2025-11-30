//go:build !windows

package restorer

import "syscall"

func notEmptyDirError() error {
	return syscall.ENOTEMPTY
}
