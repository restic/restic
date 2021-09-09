// +build !freebsd,!windows

package restic

import "golang.org/x/sys/unix"

func mknod(path string, mode uint32, dev uint64) (err error) {
	return unix.Mknod(path, mode, int(dev))
}
