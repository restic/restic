//go:build darwin || freebsd || netbsd || linux || solaris

package fs

import (
	"os"
	"slices"
	"strings"
	"sync"
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
	for _, xname := range strings.Split(os.Getenv("OS_XATTR"), ",") {
		xname = strings.TrimSpace(xname)
		if xname != "" && !slices.Contains(l, xname) {
			l = append(l, xname)
		}
	}
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

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, attr := range xattrs {
		wg.Add(1)
		go func() {
			defer wg.Done()

			attrVal, err := getxattr(path, attr)
			if err != nil {
				warnf("can not obtain extended attribute %v for %v: %v\n", attr, path, err)
				return
			}

			mu.Lock()
			node.ExtendedAttributes = append(node.ExtendedAttributes, data.ExtendedAttribute{
				Name:  attr,
				Value: attrVal,
			})
			mu.Unlock()
		}()
	}

	wg.Wait()

	slices.SortFunc(node.ExtendedAttributes, func(a, b data.ExtendedAttribute) int {
		return strings.Compare(a.Name, b.Name)
	})

	return nil
}
