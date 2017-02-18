// +build darwin

package xattr

import (
	"syscall"
	"unsafe"
)

func getxattr(path string, name string, value *byte, size int, pos int, options int) (int, error) {

	r0, _, e1 := syscall.Syscall6(syscall.SYS_GETXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))), uintptr(unsafe.Pointer(syscall.StringBytePtr(name))), uintptr(unsafe.Pointer(value)), uintptr(size), uintptr(pos), uintptr(options))
	if e1 != syscall.Errno(0) {
		return int(r0), e1
	}
	return int(r0), nil
}

func listxattr(path string, namebuf *byte, size int, options int) (int, error) {
	r0, _, e1 := syscall.Syscall6(syscall.SYS_LISTXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))), uintptr(unsafe.Pointer(namebuf)), uintptr(size), uintptr(options), 0, 0)
	if e1 != syscall.Errno(0) {
		return int(r0), e1
	}
	return int(r0), nil
}

func setxattr(path string, name string, value *byte, size int, pos int, options int) error {
	if _, _, e1 := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))), uintptr(unsafe.Pointer(syscall.StringBytePtr(name))), uintptr(unsafe.Pointer(value)), uintptr(size), uintptr(pos), uintptr(options)); e1 != syscall.Errno(0) {
		return e1
	}
	return nil
}

func removexattr(path string, name string, options int) error {
	if _, _, e1 := syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))), uintptr(unsafe.Pointer(syscall.StringBytePtr(name))), uintptr(options)); e1 != syscall.Errno(0) {
		return e1
	}
	return nil
}
