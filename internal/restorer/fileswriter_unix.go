//go:build !windows
// +build !windows

package restorer

import (
	"os"
	"syscall"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

// OpenFile opens the file with create, truncate and write only options if
// createSize is specified greater than 0 i.e. if the file hasn't already
// been created. Otherwise it opens the file with only write only option.
func (fw *filesWriter) OpenFile(createSize int64, path string, fileInfo *fileInfo) (file *os.File, err error) {
	return fw.openFile(createSize, path, fileInfo)
}

// OpenFile opens the file with create, truncate and write only options if
// createSize is specified greater than 0 i.e. if the file hasn't already
// been created. Otherwise it opens the file with only write only option.
func (fw *filesWriter) openFile(createSize int64, path string, _ *fileInfo) (file *os.File, err error) {
	if createSize >= 0 {
		file, err = openFileWithCreate(path)
		if fs.IsAccessDenied(err) {
			// If file is readonly, clear the readonly flag by resetting the
			// permissions of the file and try again
			// as the metadata will be set again in the second pass and the
			// readonly flag will be applied again if needed.
			err = fs.ResetPermissions(path)
			if err != nil {
				return nil, err
			}
			file, err = openFileWithTruncWrite(path)
		}
	} else {
		flags := os.O_WRONLY
		file, err = os.OpenFile(path, flags, 0600)
	}
	return file, err
}

// openFileWithCreate opens the file with os.O_CREATE flag along with os.O_TRUNC and os.O_WRONLY.
func openFileWithCreate(path string) (file *os.File, err error) {
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	return os.OpenFile(path, flags, 0600)
}

// openFileWithTruncWrite opens the file without os.O_CREATE flag along with os.O_TRUNC and os.O_WRONLY.
func openFileWithTruncWrite(path string) (file *os.File, err error) {
	flags := os.O_TRUNC | os.O_WRONLY
	return os.OpenFile(path, flags, 0600)
}

// CleanupPath performs clean up for the specified path.
func CleanupPath(_ string) {
	// no-op
}

func createOrOpenFile(path string, createSize int64, fileInfo *fileInfo, allowRecursiveDelete bool) (*os.File, error) {
	if createSize >= 0 {
		return createFile(path, createSize, fileInfo, allowRecursiveDelete)
	}
	return openFile(path)
}

func createFile(path string, createSize int64, fileInfo *fileInfo, allowRecursiveDelete bool) (*os.File, error) {
	f, err := fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
	if err != nil && fs.IsAccessDenied(err) {
		// If file is readonly, clear the readonly flag by resetting the
		// permissions of the file and try again
		// as the metadata will be set again in the second pass and the
		// readonly flag will be applied again if needed.
		if err = fs.ResetPermissions(path); err != nil {
			return nil, err
		}
		if f, err = fs.OpenFile(path, fs.O_WRONLY|fs.O_NOFOLLOW, 0600); err != nil {
			return nil, err
		}
	} else if err != nil && (errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EISDIR)) {
		// symlink or directory, try to remove it later on
		f = nil
	} else if err != nil {
		return nil, err
	}
	return postCreateFile(f, path, createSize, allowRecursiveDelete, fileInfo.sparse)
}
