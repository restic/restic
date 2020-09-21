package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// fixpath returns an absolute path on windows, so restic can open long file
// names.
func fixpath(name string) string {
	abspath, err := filepath.Abs(name)
	if err == nil {
		// Check if \\?\UNC\ already exist
		if strings.HasPrefix(abspath, `\\?\UNC\`) {
			return abspath
		}
		// Check if \\?\ already exist
		if strings.HasPrefix(abspath, `\\?\`) {
			return abspath
		}
		// Check if path starts with \\
		if strings.HasPrefix(abspath, `\\`) {
			return strings.Replace(abspath, `\\`, `\\?\UNC\`, 1)
		}
		// Normal path
		return `\\?\` + abspath
	}
	return name
}

// TempFile creates a temporary file.
func TempFile(dir, prefix string) (f *os.File, err error) {
	return ioutil.TempFile(dir, prefix)
}

// Chmod changes the mode of the named file to mode.
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(fixpath(name), mode)
}
