package xattr

import (
	"syscall"
	"unsafe"
)

func get(path, attr string, buf []byte) (rs int, err error) {
	return getxattr(path, attr, buf, 0, 0)
}

// getxattr retrieves value of the extended attribute identified by attr
// associated with given path in filesystem into buffer buf.
//
// options specify options for retrieving extended attributes:
// - syscall.XATTR_NOFOLLOW
// - syscall.XATTR_SHOWCOMPRESSION
//
// position should be zero. For advanded usage see getxattr(2).
//
// On success, buf contains data associated with attr, retrieved value size sz
// and nil error returned.
//
// On error, non-nil error returned. It returns error if buf was to small.
//
// A nil slice can be passed as buf to get current size of attribute value,
// which can be used to estimate buf length for value associated with attr.
//
// See getxattr(2) for more details.
//
// ssize_t getxattr(const char *path, const char *name, void *value, size_t size, u_int32_t position, int options);
func getxattr(path, name string, buf []byte, position, options int) (sz int, err error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return
	}

	n, err := syscall.BytePtrFromString(name)
	if err != nil {
		return
	}

	var b *byte
	if len(buf) > 0 {
		b = &buf[0]
	}

	r0, _, e1 := syscall.Syscall6(syscall.SYS_GETXATTR,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(n)),
		uintptr(unsafe.Pointer(b)),
		uintptr(len(buf)),
		uintptr(position),
		uintptr(options))

	sz = int(r0)
	if e1 != 0 {
		err = e1
	}

	return
}

func list(path string, dest []byte) (sz int, err error) {
	return listxattr(path, dest, 0)
}

// ssize_t listxattr(const char *path, char *namebuf, size_t size, int options);
func listxattr(path string, buf []byte, options int) (sz int, err error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return
	}

	var b *byte
	if len(buf) > 0 {
		b = &buf[0]
	}

	r0, _, e1 := syscall.Syscall6(syscall.SYS_LISTXATTR,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(b)),
		uintptr(len(buf)),
		uintptr(options), 0, 0)

	sz = int(r0)
	if e1 != 0 {
		err = e1
	}

	return
}

func set(path, attr string, data []byte, flags int) error {
	return setxattr(path, attr, data, 0, flags)
}

// int setxattr(const char *path, const char *name, void *value, size_t size, u_int32_t position, int options);
func setxattr(path string, name string, data []byte, position, options int) (err error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return
	}

	n, err := syscall.BytePtrFromString(name)
	if err != nil {
		return
	}

	var b *byte
	if len(data) > 0 {
		b = &data[0]
	}

	_, _, e1 := syscall.Syscall6(syscall.SYS_SETXATTR,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(n)),
		uintptr(unsafe.Pointer(b)),
		uintptr(len(data)),
		uintptr(position),
		uintptr(options))

	if e1 != 0 {
		err = e1
	}

	return
}

func remove(path, attr string) error {
	return removexattr(path, attr, 0)
}

// int removexattr(const char *path, const char *name, int options);
func removexattr(path string, name string, options int) (err error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return
	}

	n, err := syscall.BytePtrFromString(name)
	if err != nil {
		return
	}

	_, _, e1 := syscall.Syscall(syscall.SYS_REMOVEXATTR,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(n)),
		uintptr(options))

	if e1 != 0 {
		err = e1
	}

	return
}
