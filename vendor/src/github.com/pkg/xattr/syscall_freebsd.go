// +build freebsd

package xattr

import (
	"syscall"
	"unsafe"
)

/*
   ssize_t
   extattr_get_file(const char *path, int attrnamespace,
       const char *attrname, void *data, size_t nbytes);

   ssize_t
   extattr_set_file(const char *path, int attrnamespace,
       const char *attrname, const void *data, size_t nbytes);

   int
   extattr_delete_file(const char *path, int attrnamespace,
       const char *attrname);

   ssize_t
   extattr_list_file(const char *path, int attrnamespace, void *data,
       size_t nbytes);
*/

func extattr_get_file(path string, attrnamespace int, attrname string, data *byte, nbytes int) (int, error) {
	r, _, e := syscall.Syscall6(
		syscall.SYS_EXTATTR_GET_FILE,
		uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(attrnamespace),
		uintptr(unsafe.Pointer(syscall.StringBytePtr(attrname))),
		uintptr(unsafe.Pointer(data)),
		uintptr(nbytes),
		0,
	)
	var err error
	if e != 0 {
		err = e
	}
	return int(r), err
}

func extattr_set_file(path string, attrnamespace int, attrname string, data *byte, nbytes int) (int, error) {
	r, _, e := syscall.Syscall6(
		syscall.SYS_EXTATTR_SET_FILE,
		uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(attrnamespace),
		uintptr(unsafe.Pointer(syscall.StringBytePtr(attrname))),
		uintptr(unsafe.Pointer(data)),
		uintptr(nbytes),
		0,
	)
	var err error
	if e != 0 {
		err = e
	}
	return int(r), err
}

func extattr_delete_file(path string, attrnamespace int, attrname string) error {
	_, _, e := syscall.Syscall(
		syscall.SYS_EXTATTR_DELETE_FILE,
		uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(attrnamespace),
		uintptr(unsafe.Pointer(syscall.StringBytePtr(attrname))),
	)
	var err error
	if e != 0 {
		err = e
	}
	return err
}

func extattr_list_file(path string, attrnamespace int, data *byte, nbytes int) (int, error) {
	r, _, e := syscall.Syscall6(
		syscall.SYS_EXTATTR_LIST_FILE,
		uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
		uintptr(attrnamespace),
		uintptr(unsafe.Pointer(data)),
		uintptr(nbytes),
		0,
		0,
	)
	var err error
	if e != 0 {
		err = e
	}
	return int(r), err
}
