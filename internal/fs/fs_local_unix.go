//go:build !windows
// +build !windows

package fs

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func (f *localFile) GetBlockDeviceSize() (uint64, error) {
	var sizeBytes uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&sizeBytes)))
	if errno != 0 {
		return 0, errno
	}
	return sizeBytes, nil
}
