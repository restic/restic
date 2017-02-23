// +build !openbsd
// +build !windows

package restic

import (
	"restic/errors"
	"syscall"

	"github.com/pkg/xattr"
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	b, e := xattr.Getxattr(path, name)
	if err, ok := e.(*xattr.XAttrError); ok && err.Err == syscall.ENOTSUP {
		return nil, nil
	}
	return b, errors.Wrap(e, "Getxattr")
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	s, e := xattr.Listxattr(path)
	if err, ok := e.(*xattr.XAttrError); ok && err.Err == syscall.ENOTSUP {
		return nil, nil
	}
	return s, errors.Wrap(e, "Listxattr")
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	e := xattr.Setxattr(path, name, data)
	if err, ok := e.(*xattr.XAttrError); ok && err.Err == syscall.ENOTSUP {
		return nil
	}
	return errors.Wrap(e, "Setxattr")
}
