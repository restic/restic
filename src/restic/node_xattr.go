// +build !openbsd
// +build !windows

package restic

import (
	"github.com/pkg/xattr"
)

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	return xattr.Getxattr(path, name)
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	return xattr.Listxattr(path)
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	return xattr.Setxattr(path, name, data)
}
