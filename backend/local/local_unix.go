// +build !windows

package local

import (
	"os"
)

// set file to readonly
func setNewFileMode(f string, fi os.FileInfo) error {
	return os.Chmod(f, fi.Mode()&os.FileMode(^uint32(0222)))
}
