// +build !openbsd
// +build !windows
// +build !freebsd

package restic

import (
	"github.com/ivaxer/go-xattr"
	"syscall"
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	b, e := xattr.Get(path, name)
	if e == syscall.ENOTSUP {
		return nil, nil
	}
	return b, e
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	s, e := xattr.List(path)
	if e == syscall.ENOTSUP {
		return nil, nil
	}
	return s, e
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	e := xattr.Set(path, name, data)
	if e == syscall.ENOTSUP {
		return nil
	}
	return e
}
