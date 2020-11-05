// +build !windows

package fs

import (
	"io/ioutil"
	"os"
	"syscall"

	"github.com/restic/restic/internal/errors"
)

// fixpath returns an absolute path on windows, so restic can open long file
// names.
func fixpath(name string) string {
	return name
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

var osChmod = os.Chmod // Reset by test.

// Chmod changes the mode of the named file to mode.
func Chmod(name string, mode os.FileMode) (err error) {
	const maxTries = 100 // Arbitrary.

	for try := 0; try < maxTries; try++ {
		err = osChmod(name, mode)
		if e, ok := err.(*os.PathError); ok {
			switch e.Err {
			case syscall.EINTR:
				// CIFS (and maybe fuse?) on Linux can return EINTR. Retry.
				continue
			case syscall.ENOTSUP:
				// CIFS with gvfs on Linux does not support chmod. Ignore.
				return nil
			}
		}
		return err
	}

	return errors.Wrap(err, "max. number of tries exceeded")
}
