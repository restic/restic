package fs_test

import (
	iofs "io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestRecallOnDataAccessRealFile(t *testing.T) {
	// create a temp file for testing
	tempdir := rtest.TempDir(t)
	filename := filepath.Join(tempdir, "regular-file")
	err := os.WriteFile(filename, []byte("foobar"), 0640)
	rtest.OK(t, err)

	fi, err := os.Stat(filename)
	rtest.OK(t, err)

	xs := fs.ExtendedStat(fi)

	// ensure we can check attrs without error
	recall, err := xs.RecallOnDataAccess()
	rtest.Assert(t, err == nil, "err should be nil", err)
	rtest.Assert(t, recall == false, "RecallOnDataAccess should be false")
}

// mockFileInfo implements os.FileInfo for mocking file attributes
type mockFileInfo struct {
	FileAttributes uint32
}

func (m mockFileInfo) IsDir() bool {
	return false
}
func (m mockFileInfo) ModTime() time.Time {
	return time.Now()
}
func (m mockFileInfo) Mode() iofs.FileMode {
	return 0
}
func (m mockFileInfo) Name() string {
	return "test"
}
func (m mockFileInfo) Size() int64 {
	return 0
}
func (m mockFileInfo) Sys() any {
	return &syscall.Win32FileAttributeData{
		FileAttributes: m.FileAttributes,
	}
}

func TestRecallOnDataAccessMockCloudFile(t *testing.T) {
	fi := mockFileInfo{
		FileAttributes: windows.FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS,
	}
	xs := fs.ExtendedStat(fi)

	recall, err := xs.RecallOnDataAccess()
	rtest.Assert(t, err == nil, "err should be nil", err)
	rtest.Assert(t, recall, "RecallOnDataAccess should be true")
}

func TestRecallOnDataAccessMockRegularFile(t *testing.T) {
	fi := mockFileInfo{
		FileAttributes: windows.FILE_ATTRIBUTE_ARCHIVE,
	}
	xs := fs.ExtendedStat(fi)

	recall, err := xs.RecallOnDataAccess()
	rtest.Assert(t, err == nil, "err should be nil", err)
	rtest.Assert(t, recall == false, "RecallOnDataAccess should be false")
}
