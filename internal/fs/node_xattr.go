//go:build darwin || freebsd || netbsd || linux || solaris

package fs

import (
	"bytes"
	"os"
	"syscall"

	"github.com/restic/restic/internal/data"
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
		// ENOATTR instead of ENOTSUP.  BSD can return EOPNOTSUPP.
		if e.Err == syscall.ENOTSUP || e.Err == syscall.EOPNOTSUPP || e.Err == xattr.ENOATTR {
			return nil
		}
		return errors.WithStack(e)

	default:
		return errors.WithStack(e)
	}
}

func nodeRestoreExtendedAttributes(node *data.Node, path string, xattrSelectFilter func(xattrName string) bool) error {
	expectedAttrs := map[string]struct{}{}
	for _, attr := range node.ExtendedAttributes {
		// Only restore xattrs that match the filter
		if xattrSelectFilter(attr.Name) {
			err := setxattr(path, attr.Name, attr.Value)
			if err != nil {
				return err
			}
			expectedAttrs[attr.Name] = struct{}{}
		}
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
		// Only attempt to remove xattrs that match the filter
		if xattrSelectFilter(name) {
			if err := removexattr(path, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// nodeXattrsNeedRestore reports whether the extended attributes of the file at
// path differ from the ones stored in node in a way that
// nodeRestoreExtendedAttributes would change.
func nodeXattrsNeedRestore(node *data.Node, path string, fi *ExtendedFileInfo, xattrSelectFilter func(xattrName string) bool) (differs, appliable bool) {
	expectedAttrs := make(map[string][]byte)
	for _, attr := range node.ExtendedAttributes {
		if xattrSelectFilter(attr.Name) {
			expectedAttrs[attr.Name] = attr.Value
		}
	}

	names, err := listxattr(path)
	if err != nil {
		// cannot determine the current attributes
		return true, true
	}

	for _, name := range names {
		if !xattrSelectFilter(name) {
			continue
		}
		value, ok := expectedAttrs[name]
		if !ok {
			// unexpected attribute which would be removed
			return true, canModifyMetadata(fi)
		}
		current, err := getxattr(path, name)
		if err != nil || !bytes.Equal(current, value) {
			return true, canModifyMetadata(fi)
		}
		delete(expectedAttrs, name)
	}

	// missing attributes which would be set
	return len(expectedAttrs) > 0, canModifyMetadata(fi)
}

func nodeFillExtendedAttributes(node *data.Node, path string, ignoreListError bool, warnf func(format string, args ...any)) error {
	xattrs, err := listxattr(path)
	debug.Log("fillExtendedAttributes(%v) %v %v", path, xattrs, err)
	if err != nil {
		if ignoreListError && isListxattrPermissionError(err) {
			return nil
		}
		return err
	}

	node.ExtendedAttributes = make([]data.ExtendedAttribute, 0, len(xattrs))
	for _, attr := range xattrs {
		attrVal, err := getxattr(path, attr)
		if err != nil {
			warnf("can not obtain extended attribute %v for %v: %v\n", attr, path, err)
			continue
		}
		attr := data.ExtendedAttribute{
			Name:  attr,
			Value: attrVal,
		}

		node.ExtendedAttributes = append(node.ExtendedAttributes, attr)
	}

	return nil
}
