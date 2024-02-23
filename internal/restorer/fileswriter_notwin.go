//go:build !windows
// +build !windows

package restorer

import "os"

// OpenFile opens the file with create, truncate and write only options if
// createSize is specified greater than 0 i.e. if the file hasn't already
// been created. Otherwise it opens the file with only write only option.
func (*filesWriter) OpenFile(createSize int64, path string) (*os.File, error) {
	var flags int
	if createSize >= 0 {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	} else {
		flags = os.O_WRONLY
	}

	f, err := os.OpenFile(path, flags, 0600)
	return f, err
}
