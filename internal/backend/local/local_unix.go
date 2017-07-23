// +build !windows

package local

import (
	"os"

	"github.com/restic/restic/internal/fs"
)

// set file to readonly
func setNewFileMode(f string, fi os.FileInfo) error {
	return fs.Chmod(f, fi.Mode()&os.FileMode(^uint32(0222)))
}
