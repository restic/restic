// +build linux

package xattr

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	XATTR_CREATE  = unix.XATTR_CREATE
	XATTR_REPLACE = unix.XATTR_REPLACE

	// ENOATTR is not exported by the syscall package on Linux, because it is
	// an alias for ENODATA. We export it here so it is available on all
	// our supported platforms.
	ENOATTR = syscall.ENODATA
)

func getxattr(path string, name string, data []byte) (int, error) {
	return unix.Getxattr(path, name, data)
}

func lgetxattr(path string, name string, data []byte) (int, error) {
	return unix.Lgetxattr(path, name, data)
}

func fgetxattr(f *os.File, name string, data []byte) (int, error) {
	return unix.Fgetxattr(int(f.Fd()), name, data)
}

func setxattr(path string, name string, data []byte, flags int) error {
	return unix.Setxattr(path, name, data, flags)
}

func lsetxattr(path string, name string, data []byte, flags int) error {
	return unix.Lsetxattr(path, name, data, flags)
}

func fsetxattr(f *os.File, name string, data []byte, flags int) error {
	return unix.Fsetxattr(int(f.Fd()), name, data, flags)
}

func removexattr(path string, name string) error {
	return unix.Removexattr(path, name)
}

func lremovexattr(path string, name string) error {
	return unix.Lremovexattr(path, name)
}

func fremovexattr(f *os.File, name string) error {
	return unix.Fremovexattr(int(f.Fd()), name)
}

func listxattr(path string, data []byte) (int, error) {
	return unix.Listxattr(path, data)
}

func llistxattr(path string, data []byte) (int, error) {
	return unix.Llistxattr(path, data)
}

func flistxattr(f *os.File, data []byte) (int, error) {
	return unix.Flistxattr(int(f.Fd()), data)
}

// stringsFromByteSlice converts a sequence of attributes to a []string.
// On Darwin and Linux, each entry is a NULL-terminated string.
func stringsFromByteSlice(buf []byte) (result []string) {
	offset := 0
	for index, b := range buf {
		if b == 0 {
			result = append(result, string(buf[offset:index]))
			offset = index + 1
		}
	}
	return
}
