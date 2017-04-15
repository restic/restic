// +build !windows

package fs

import "os"

// fixpath returns an absolute path on windows, so restic can open long file
// names.
func fixpath(name string) string {
	return name
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is already a directory,
// MkdirAll does nothing and returns nil.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(fixpath(path), perm)
}
