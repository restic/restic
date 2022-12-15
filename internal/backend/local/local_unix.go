//go:build !windows
// +build !windows

package local

import (
	"errors"
	"os"
	"runtime"
	"syscall"

	"github.com/restic/restic/internal/fs"
)

// fsyncDir flushes changes to the directory dir.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}

	err = d.Sync()
	if err != nil &&
		(errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOENT) ||
			errors.Is(err, syscall.EINVAL) || isMacENOTTY(err)) {
		err = nil
	}

	cerr := d.Close()
	if err == nil {
		err = cerr
	}

	return err
}

// The ExFAT driver on some versions of macOS can return ENOTTY,
// "inappropriate ioctl for device", for fsync.
//
// https://github.com/restic/restic/issues/4016
// https://github.com/realm/realm-core/issues/5789
func isMacENOTTY(err error) bool {
	return runtime.GOOS == "darwin" && errors.Is(err, syscall.ENOTTY)
}

// set file to readonly
func setFileReadonly(f string, mode os.FileMode) error {
	return fs.Chmod(f, mode&^0222)
}
