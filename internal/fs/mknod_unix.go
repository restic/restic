//go:build !freebsd && !windows
// +build !freebsd,!windows

package fs

import "golang.org/x/sys/unix"

func mknod(path string, mode uint32, dev uint64) (err error) {
	return unix.Mknod(path, mode, int(dev))
}
