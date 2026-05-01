//go:build darwin

package archiver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
)

func TestRejectMacOSBackupExcludesWithChecker(t *testing.T) {
	var calls atomic.Int32
	reject := rejectMacOSBackupExcludesWithChecker(func(path string, isDir bool) (bool, error) {
		calls.Add(1)
		rtest.Assert(t, isDir, "expected directory flag")
		return strings.HasSuffix(path, "excluded"), nil
	}, t.Logf)

	dirInfo, err := fs.Local{}.Lstat(t.TempDir())
	rtest.OK(t, err)

	rtest.Assert(t, !reject("/tmp/included", dirInfo, fs.Local{}), "included item was rejected")
	rtest.Assert(t, reject("/tmp/excluded", dirInfo, fs.Local{}), "excluded item was included")
	rtest.Assert(t, reject("/tmp/excluded", dirInfo, fs.Local{}), "cached excluded item was included")
	rtest.Equals(t, int32(2), calls.Load())
}

func TestRejectMacOSBackupExcludesWarnsAndIncludesOnError(t *testing.T) {
	var warning string
	reject := rejectMacOSBackupExcludesWithChecker(func(string, bool) (bool, error) {
		return false, errors.New("boom")
	}, func(msg string, args ...interface{}) {
		warning = msg
	})

	file, err := os.CreateTemp("", "restic-macos-backup-exclude-")
	rtest.OK(t, err)
	defer func() {
		_ = os.Remove(file.Name())
	}()
	rtest.OK(t, file.Close())

	fileInfo, err := fs.Local{}.Lstat(file.Name())
	rtest.OK(t, err)

	rtest.Assert(t, !reject(file.Name(), fileInfo, fs.Local{}), "item with check error was rejected")
	rtest.Assert(t, strings.Contains(warning, "macOS backup exclusion"), "unexpected warning %q", warning)
}

func TestBackupCoreStickyExclusion(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "excluded")
	rtest.OK(t, os.WriteFile(filename, []byte("test"), 0600))

	excluded, err := backupCoreIsItemExcluded(filename, false)
	rtest.OK(t, err)
	rtest.Assert(t, !excluded, "new file is unexpectedly excluded")

	rtest.OK(t, setMacOSBackupExcluded(filename, false, true, false))
	defer func() {
		rtest.OK(t, setMacOSBackupExcluded(filename, false, false, false))
	}()

	excluded, err = backupCoreIsItemExcluded(filename, false)
	rtest.OK(t, err)
	rtest.Assert(t, excluded, "Backup Core did not report sticky exclusion")

	reject, err := RejectMacOSBackupExcludes(t.Logf)
	rtest.OK(t, err)

	fileInfo, err := fs.Local{}.Lstat(filename)
	rtest.OK(t, err)
	rtest.Assert(t, reject(filename, fileInfo, fs.Local{}), "Backup Core excluded file was included")
}

func setMacOSBackupExcluded(path string, isDir bool, exclude bool, excludeByPath bool) error {
	handle, err := purego.Dlopen("/System/Library/Frameworks/CoreServices.framework/CoreServices", rtldLazy|rtldLocal)
	if err != nil {
		return err
	}

	var csBackupSetItemExcluded func(item uintptr, exclude byte, excludeByPath byte) int32
	if err := registerLibFunc(&csBackupSetItemExcluded, handle, "CSBackupSetItemExcluded"); err != nil {
		return err
	}

	url, err := backupCoreFileURL(path, isDir)
	if err != nil {
		return err
	}
	defer cfRelease(url)

	excludeByte := byte(0)
	if exclude {
		excludeByte = 1
	}

	excludeByPathByte := byte(0)
	if excludeByPath {
		excludeByPathByte = 1
	}

	if status := csBackupSetItemExcluded(url, excludeByte, excludeByPathByte); status != 0 {
		return fmt.Errorf("CSBackupSetItemExcluded returned OSStatus %d", status)
	}
	return nil
}
