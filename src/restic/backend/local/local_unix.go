// +build !windows

package local

import (
	"os"
	"restic/fs"
	"syscall"
)

// set file to readonly
func setNewFileMode(f string, fi os.FileInfo) error {
	err := fs.Chmod(f, fi.Mode()&os.FileMode(^uint32(0222)))
	// ignore the error if the FS does not support setting this mode (e.g. CIFS with gvfs on Linux)
	if perr, ok := err.(*os.PathError); ok && perr.Err == syscall.ENOTSUP {
		err = nil
	}
	return err
}
