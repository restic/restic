//go:build darwin

package archiver

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

const (
	rtldLazy  = 1
	rtldLocal = 4
)

var (
	backupCoreOnce    sync.Once
	backupCoreLoadErr error

	cfURLCreateFromFileSystemRepresentation func(allocator uintptr, buffer *byte, bufLen int64, isDirectory byte) uintptr
	cfRelease                               func(cf uintptr)
	csBackupIsItemExcluded                  func(item uintptr, byPath *byte) byte

	macOSBackupExcluded = backupCoreIsItemExcluded
)

func loadBackupCore() error {
	backupCoreOnce.Do(func() {
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", rtldLazy|rtldLocal)
		if err != nil {
			backupCoreLoadErr = fmt.Errorf("open CoreFoundation: %w", err)
			return
		}

		cs, err := purego.Dlopen("/System/Library/Frameworks/CoreServices.framework/CoreServices", rtldLazy|rtldLocal)
		if err != nil {
			backupCoreLoadErr = fmt.Errorf("open CoreServices: %w", err)
			return
		}

		if err := registerLibFunc(&cfURLCreateFromFileSystemRepresentation, cf, "CFURLCreateFromFileSystemRepresentation"); err != nil {
			backupCoreLoadErr = err
			return
		}
		if err := registerLibFunc(&cfRelease, cf, "CFRelease"); err != nil {
			backupCoreLoadErr = err
			return
		}
		if err := registerLibFunc(&csBackupIsItemExcluded, cs, "CSBackupIsItemExcluded"); err != nil {
			backupCoreLoadErr = err
			return
		}
	})

	return backupCoreLoadErr
}

func registerLibFunc(fptr any, handle uintptr, name string) error {
	fn, err := purego.Dlsym(handle, name)
	if err != nil {
		return fmt.Errorf("load %s: %w", name, err)
	}
	purego.RegisterFunc(fptr, fn)
	return nil
}

func backupCoreFileURL(path string, isDir bool) (uintptr, error) {
	if err := loadBackupCore(); err != nil {
		return 0, err
	}

	buf := append([]byte(path), 0)
	dir := byte(0)
	if isDir {
		dir = 1
	}

	url := cfURLCreateFromFileSystemRepresentation(0, &buf[0], int64(len(buf)-1), dir)
	if url == 0 {
		return 0, fmt.Errorf("CFURLCreateFromFileSystemRepresentation returned NULL for %q", path)
	}
	return url, nil
}

func backupCoreIsItemExcluded(path string, isDir bool) (bool, error) {
	url, err := backupCoreFileURL(path, isDir)
	if err != nil {
		return false, err
	}
	defer cfRelease(url)

	var byPath byte
	excluded := csBackupIsItemExcluded(url, &byPath) != 0
	return excluded, nil
}

// RejectMacOSBackupExcludes returns a reject function for items marked as
// excluded from macOS backups via Backup Core.
func RejectMacOSBackupExcludes(warnf func(msg string, args ...interface{})) (RejectFunc, error) {
	if err := loadBackupCore(); err != nil {
		return nil, err
	}

	return rejectMacOSBackupExcludesWithChecker(macOSBackupExcluded, warnf), nil
}

func rejectMacOSBackupExcludesWithChecker(checker func(path string, isDir bool) (bool, error), warnf func(msg string, args ...interface{})) RejectFunc {
	var cache sync.Map
	return func(item string, fi *fs.ExtendedFileInfo, _ fs.FS) bool {
		if cached, ok := cache.Load(item); ok {
			return cached.(bool)
		}

		excluded, err := checker(item, fi.Mode.IsDir())
		if err != nil {
			if warnf != nil {
				warnf("item %v: error checking macOS backup exclusion status: %v\n", item, err)
			}
			cache.Store(item, false)
			return false
		}

		if excluded {
			debug.Log("rejecting macOS backup excluded item %s", item)
		}
		cache.Store(item, excluded)
		return excluded
	}
}
