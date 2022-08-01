//go:build darwin || freebsd || linux || solaris
// +build darwin freebsd linux solaris

package restic

import (
	"syscall"

	"github.com/restic/restic/internal/errors"

	"github.com/pkg/xattr"
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	b, err := xattr.Get(path, name)
	return b, handleXattrErr(err)
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	l, err := xattr.List(path)
	return l, handleXattrErr(err)
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	return handleXattrErr(xattr.Set(path, name, data))
}

func handleXattrErr(err error) error {
	switch e := err.(type) {
	case nil:
		return nil

	case *xattr.Error:
		// On Solaris, xattr not being supported on a file is signaled
		// by EINVAL (https://github.com/pkg/xattr/issues/67).
		// On Linux, xattr calls on files in an SMB/CIFS mount can return
		// ENOATTR instead of ENOTSUP.
		switch e.Err {
		case syscall.EINVAL, syscall.ENOTSUP, xattr.ENOATTR:
			return nil
		}
		return errors.WithStack(e)

	default:
		return errors.WithStack(e)
	}
}
