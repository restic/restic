//go:build !windows

package fileio

import (
	"os"
)

// TempFile creates a temporary file which has already been deleted (on
// supported platforms)
func TempFile(dir, prefix string) (f *os.File, err error) {
	f, err = os.CreateTemp(dir, prefix)
	if err != nil {
		return nil, err
	}

	if err = os.Remove(f.Name()); err != nil {
		return nil, err
	}

	return f, nil
}
