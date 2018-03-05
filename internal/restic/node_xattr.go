// +build !openbsd
// +build !solaris
// +build !windows

package restic

import (
	"syscall"

	"github.com/restic/restic/internal/errors"

	"github.com/pkg/xattr"
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	b, e := xattr.Get(path, name)
	if err, ok := e.(*xattr.Error); ok && err.Err == syscall.ENOTSUP {
		return nil, nil
	}
	return b, errors.Wrap(e, "Getxattr")
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	s, e := xattr.List(path)
	if err, ok := e.(*xattr.Error); ok && err.Err == syscall.ENOTSUP {
		return nil, nil
	}
	return s, errors.Wrap(e, "Listxattr")
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	e := xattr.Set(path, name, data)
	if err, ok := e.(*xattr.Error); ok && err.Err == syscall.ENOTSUP {
		return nil
	}
	return errors.Wrap(e, "Setxattr")
}
