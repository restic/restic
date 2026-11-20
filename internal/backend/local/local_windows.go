package local

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/restic/restic/internal/errors"
)

// fsyncDir flushes directory changes so the directory entry of files created
// or renamed inside dir survives a power failure, like fsync(dir) on POSIX.
//
// The handle must have GENERIC_WRITE: FlushFileBuffers on a handle opened for
// listing (GENERIC_READ) fails with ERROR_ACCESS_DENIED.
// FILE_FLAG_BACKUP_SEMANTICS is what makes it possible to open a directory at
// all. No administrator privilege is involved.
func fsyncDir(dir string) error {
	pathp, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return errors.WithStack(err)
	}

	h, err := windows.CreateFile(
		pathp,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		_ = windows.CloseHandle(h)
	}()

	if err := windows.FlushFileBuffers(h); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Windows is not macOS.
func isMacENOTTY(_ error) bool { return false }

// We don't modify read-only on windows,
// since it will make us unable to delete the file,
// and this isn't common practice on this platform.
func setFileReadonly(_ string, _ os.FileMode) error {
	return nil
}

func removeFile(f string) error {
	// Reset read-only flag,
	// as Windows won't let you delete a read-only file
	err := os.Chmod(f, 0666)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return os.Remove(f)
}
