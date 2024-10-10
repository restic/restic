//go:build freebsd
// +build freebsd

package fs

import "syscall"

func mknod(path string, mode uint32, dev uint64) (err error) {
	return syscall.Mknod(path, mode, dev)
}
