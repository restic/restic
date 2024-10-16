//go:build !freebsd && !windows
// +build !freebsd,!windows

package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

func mknod(path string, mode uint32, dev uint64) error {
	err := unix.Mknod(path, mode, int(dev))
	if err != nil {
		err = &os.PathError{Op: "mknod", Path: path, Err: err}
	}
	return err
}
