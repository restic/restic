// +build darwin

package xattr

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// See https://opensource.apple.com/source/xnu/xnu-1504.15.3/bsd/sys/xattr.h.auto.html
const (
	XATTR_NOFOLLOW        = 0x0001
	XATTR_CREATE          = 0x0002
	XATTR_REPLACE         = 0x0004
	XATTR_NOSECURITY      = 0x0008
	XATTR_NODEFAULT       = 0x0010
	XATTR_SHOWCOMPRESSION = 0x0020

	// ENOATTR is not exported by the syscall package on Linux, because it is
	// an alias for ENODATA. We export it here so it is available on all
	// our supported platforms.
	ENOATTR = syscall.ENOATTR
)

func getxattr(path string, name string, data []byte) (int, error) {
	return unix.Getxattr(path, name, data)
}

func lgetxattr(path string, name string, data []byte) (int, error) {
	value, size := bytePtrFromSlice(data)
	/*
		ssize_t getxattr(
			const char *path,
			const char *name,
			void *value,
			size_t size,
			u_int32_t position,
			int options
		)
	*/
	r0, _, err := syscall.Syscall6(syscall.SYS_GETXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(unsafe.Pointer(syscall.StringBytePtr(name))), uintptr(unsafe.Pointer(value)), uintptr(size), 0, XATTR_NOFOLLOW)
	if err != syscall.Errno(0) {
		return int(r0), err
	}
	return int(r0), nil
}

func setxattr(path string, name string, data []byte, flags int) error {
	return unix.Setxattr(path, name, data, flags)
}

func lsetxattr(path string, name string, data []byte, flags int) error {
	return unix.Setxattr(path, name, data, flags|XATTR_NOFOLLOW)
}

func removexattr(path string, name string) error {
	return unix.Removexattr(path, name)
}

func lremovexattr(path string, name string) error {
	/*
		int removexattr(
			const char *path,
			const char *name,
			int options
		);
	*/
	_, _, err := syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(unsafe.Pointer(syscall.StringBytePtr(name))), XATTR_NOFOLLOW)
	if err != syscall.Errno(0) {
		return err
	}
	return nil
}

func listxattr(path string, data []byte) (int, error) {
	return unix.Listxattr(path, data)
}

func llistxattr(path string, data []byte) (int, error) {
	name, size := bytePtrFromSlice(data)
	/*
		ssize_t listxattr(
			const char *path,
			char *name,
			size_t size,
			int options
		);
	*/
	r0, _, err := syscall.Syscall6(syscall.SYS_LISTXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(unsafe.Pointer(name)), uintptr(size), XATTR_NOFOLLOW, 0, 0)
	if err != syscall.Errno(0) {
		return int(r0), err
	}
	return int(r0), nil
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
