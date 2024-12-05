//go:build linux
// +build linux

package fs

import (
	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/unix"
	"os"
)

func init() {
	registerCloneMethod(doCloneIoctl)
	registerCloneMethod(doCloneCopyFileRange)
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

func doCloneCopyFileRange(src, dest *os.File) (cloned bool, err error) {
	srcInfo, err := src.Stat()
	if err != nil {
		return false, err
	}
	srcLength := int(srcInfo.Size())

	n, err := withFileDescriptors(src, dest, func(srcFd, destFd int) (int, error) {
		var srcOff, destOff int64 = 0, 0
		return unix.CopyFileRange(srcFd, &srcOff, destFd, &destOff, srcLength, 0)
	})

	if n > 0 && n != srcLength {
		return false, errors.Wrapf(err, "copy_file_range() wrote %d of %d bytes", n, srcLength)
	}
	// we cannot guarantee that copy_file_range() performs block cloning
	return false, err
}
