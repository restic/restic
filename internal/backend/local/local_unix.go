// +build !windows

package local

import (
	"os"

	"github.com/restic/restic/internal/fs"
)

// set file to readonly
func setNewFileMode(f string, mode os.FileMode) error {
	return fs.Chmod(f, mode)
}
