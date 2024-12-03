//go:build linux
// +build linux

package fs

import (
	"golang.org/x/sys/unix"
	"os"
)

func init() {
	registerCloneMethod(doCloneIoctl)
}

func withFileDescriptors(file1, file2 *os.File, fn func(fd1, fd2 int) (int, error)) (int, error) {
	conn1, err := file1.SyscallConn()
	if err != nil {
		return 0, err
	}
	conn2, err := file2.SyscallConn()
	if err != nil {
		return 0, err
	}
	var n int
	var err1, err2, err3 error
	err1 = conn1.Control(func(first uintptr) {
		err2 = conn2.Control(func(second uintptr) {
			n, err3 = fn(int(first), int(second))
		})
	})
	if err1 != nil {
		return n, err1
	}
	if err2 != nil {
		return n, err2
	}
	return n, err3
}

func doCloneIoctl(src, dest *os.File) (cloned bool, err error) {
	_, err = withFileDescriptors(src, dest, func(srcFd, destFd int) (int, error) {
		return 0, unix.IoctlFileClone(destFd, srcFd)
	})
	return true, err
}
