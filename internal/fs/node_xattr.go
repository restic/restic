//go:build darwin || freebsd || linux || solaris
// +build darwin freebsd linux solaris

package fs

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/pkg/xattr"
)

func linuxFdPath(fd uintptr) string {
	// A file handle opened using O_PATH on Linux cannot be directly used to read xattrs.
	// However, the file descriptor objects in the procfs filesystem
	// can be used in place of the original file and therefore allow xattr access.
	return fmt.Sprintf("/proc/self/fd/%d", int(fd))
}

// replaceXattrErrorPath replaces the path in the given xattr.Error
// For other error types it is a no-op. This is intended to replace
// a linuxFdPath with the original path.
func replaceXattrErrorPath(err error, path string) error {
	switch e := err.(type) {
	case nil:
		return nil

	case *xattr.Error:
		e.Path = path
		return e
	default:
		return err
	}
}

// getxattr retrieves extended attribute data associated with path.
func fgetxattr(f *os.File, name string) (b []byte, err error) {
	if runtime.GOOS == "linux" {
		b, err = xattr.Get(linuxFdPath(f.Fd()), name)
		err = replaceXattrErrorPath(err, f.Name())
	} else {
		b, err = xattr.FGet(f, name)
	}
	return b, handleXattrErr(err)
}

// getxattr retrieves extended attribute data associated with path.
func getxattr(path string, name string) (b []byte, err error) {
	b, err = xattr.Get(path, name)
	return b, handleXattrErr(err)
}

// lgetxattr retrieves extended attribute data associated with path.
func lgetxattr(path string, name string) (b []byte, err error) {
	b, err = xattr.LGet(path, name)
	return b, handleXattrErr(err)
}

// flistxattr retrieves a list of names of extended attributes associated with the
// given file.
func flistxattr(f *os.File) (l []string, err error) {
	if runtime.GOOS == "linux" {
		l, err = xattr.List(linuxFdPath(f.Fd()))
		err = replaceXattrErrorPath(err, f.Name())
	} else {
		l, err = xattr.FList(f)
	}
	return l, handleXattrErr(err)
}

// listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func listxattr(path string) ([]string, error) {
	l, err := xattr.List(path)
	return l, handleXattrErr(err)
}

// llistxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func llistxattr(path string) ([]string, error) {
	l, err := xattr.LList(path)
	return l, handleXattrErr(err)
}

func isListxattrPermissionError(err error) bool {
	var xerr *xattr.Error
	if errors.As(err, &xerr) {
		return xerr.Op == "xattr.list" && errors.Is(xerr.Err, os.ErrPermission)
	}
	return false
}

// setxattr associates name and data together as an attribute of path.
func setxattr(path, name string, data []byte) error {
	return handleXattrErr(xattr.LSet(path, name, data))
}

// removexattr removes the attribute name from path.
func removexattr(path, name string) error {
	return handleXattrErr(xattr.LRemove(path, name))
}

func handleXattrErr(err error) error {
	switch e := err.(type) {
	case nil:
		return nil

	case *xattr.Error:
		// On Linux, xattr calls on files in an SMB/CIFS mount can return
		// ENOATTR instead of ENOTSUP.
		switch e.Err {
		case syscall.ENOTSUP, xattr.ENOATTR:
			return nil
		}
		return errors.WithStack(e)

	default:
		return errors.WithStack(e)
	}
}

func nodeRestoreExtendedAttributes(node *restic.Node, path string) error {
	expectedAttrs := map[string]struct{}{}
	for _, attr := range node.ExtendedAttributes {
		err := setxattr(path, attr.Name, attr.Value)
		if err != nil {
			return err
		}
		expectedAttrs[attr.Name] = struct{}{}
	}

	// remove unexpected xattrs
	xattrs, err := llistxattr(path)
	if err != nil {
		return err
	}
	for _, name := range xattrs {
		if _, ok := expectedAttrs[name]; ok {
			continue
		}
		if err := removexattr(path, name); err != nil {
			return err
		}
	}

	return nil
}

func nodeFillExtendedAttributes(node *restic.Node, meta metadataHandle, ignoreListError bool) error {
	var err error
	node.ExtendedAttributes, err = meta.Xattr(ignoreListError)
	return err
}
