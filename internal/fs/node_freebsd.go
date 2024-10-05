//go:build freebsd
// +build freebsd

package fs

import (
	"os"
	"syscall"
)

func nodeRestoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

func mknod(path string, mode uint32, dev uint64) error {
	err := syscall.Mknod(path, mode, dev)
	if err != nil {
		err = &os.PathError{Op: "mknod", Path: path, Err: err}
	}
	return err
}
