//go:build windows

package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestBackupPrivilegeBypassACL(t *testing.T) {
	testPath := testGetRestrictedFilePath(t)

	// Read-only Open/OpenFile should automatically use FILE_FLAG_BACKUP_SEMANTICS since Go v1.20.
	testfile, err := os.Open(testPath)
	test.OK(t, errors.Wrapf(err, "failed to open file for reading: %s", testPath))
	test.OK(t, testfile.Close())
}

func TestRestorePrivilegeBypassACL(t *testing.T) {
	testPath := testGetRestrictedFilePath(t)

	// Writable OpenFile needs explicit FILE_FLAG_BACKUP_SEMANTICS.
	testfile, err := os.OpenFile(testPath, os.O_WRONLY|windows.O_FILE_FLAG_BACKUP_SEMANTICS, 0)
	test.OK(t, errors.Wrapf(err, "failed to open file for writing: %s", testPath))
	test.OK(t, testfile.Close())
}

func testGetRestrictedFilePath(t *testing.T) string {
	// Non-admin is unlikely to have needed privileges.
	isAdmin, err := isAdmin()
	test.OK(t, errors.Wrap(err, "failed to check if user is admin"))
	if !isAdmin {
		t.Skip("not running with administrator access, skipping")
	}

	// Create temporary file.
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "testfile.txt")

	testfile, err := os.Create(testPath)
	test.OK(t, errors.Wrapf(err, "failed to create temporary file: %s", testPath))
	test.OK(t, testfile.Close())

	// Set restricted permissions.
	// Deny file read/write/execute to "Everyone" (all accounts); allow delete to "Everyone".
	sd, err := windows.SecurityDescriptorFromString("D:PAI(D;;FRFWFX;;;WD)(A;;SD;;;WD)")
	test.OK(t, errors.Wrap(err, "failed to parse SDDL: %s"))
	dacl, _, err := sd.DACL()
	test.OK(t, errors.Wrap(err, "failed to extract SD DACL"))
	err = windows.SetNamedSecurityInfo(testPath, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	test.OK(t, errors.Wrapf(err, "failed to set SD: %s", testPath))

	return testPath
}
