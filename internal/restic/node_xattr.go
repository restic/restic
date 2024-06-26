//go:build darwin || freebsd || linux || solaris
// +build darwin freebsd linux solaris

package restic

import (
	"fmt"
	"os"
	"syscall"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/pkg/xattr"
)

// getxattr retrieves extended attribute data associated with path.
func getxattr(path, name string) ([]byte, error) {
	b, err := xattr.LGet(path, name)
	return b, handleXattrErr(err)
}

// listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func listxattr(path string) ([]string, error) {
	l, err := xattr.LList(path)
	return l, handleXattrErr(err)
}

func IsListxattrPermissionError(err error) bool {
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

// restoreGenericAttributes is no-op.
func (node *Node) restoreGenericAttributes(_ string, warn func(msg string)) error {
	return node.handleAllUnknownGenericAttributesFound(warn)
}

// fillGenericAttributes is a no-op.
func (node *Node) fillGenericAttributes(_ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}

func (node Node) restoreExtendedAttributes(path string) error {
	expectedAttrs := map[string]struct{}{}
	for _, attr := range node.ExtendedAttributes {
		err := setxattr(path, attr.Name, attr.Value)
		if err != nil {
			return err
		}
		expectedAttrs[attr.Name] = struct{}{}
	}

	// remove unexpected xattrs
	xattrs, err := listxattr(path)
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

func (node *Node) fillExtendedAttributes(path string, ignoreListError bool) error {
	xattrs, err := listxattr(path)
	debug.Log("fillExtendedAttributes(%v) %v %v", path, xattrs, err)
	if err != nil {
		if ignoreListError && IsListxattrPermissionError(err) {
			return nil
		}
		return err
	}

	node.ExtendedAttributes = make([]ExtendedAttribute, 0, len(xattrs))
	for _, attr := range xattrs {
		attrVal, err := getxattr(path, attr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "can not obtain extended attribute %v for %v:\n", attr, path)
			continue
		}
		attr := ExtendedAttribute{
			Name:  attr,
			Value: attrVal,
		}

		node.ExtendedAttributes = append(node.ExtendedAttributes, attr)
	}

	return nil
}
