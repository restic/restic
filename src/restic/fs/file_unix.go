// +build !windows

package fs

import (
	"io/ioutil"
	"os"
)

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

// TempFile creates a temporary file which has already been deleted (on
// supported platforms)
func TempFile(dir, prefix string) (f *os.File, err error) {
	f, err = ioutil.TempFile(dir, prefix)
	if err != nil {
		return nil, err
	}

	if err = os.Remove(f.Name()); err != nil {
		return nil, err
	}

	return f, nil
}
