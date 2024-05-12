//go:build windows
// +build windows

package fs

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
)

func TestSetGetFileSecurityDescriptors(t *testing.T) {
	tempDir := t.TempDir()
	testfilePath := filepath.Join(tempDir, "testfile.txt")
	// create temp file
	testfile, err := os.Create(testfilePath)
	if err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}
	defer func() {
		err := testfile.Close()
		if err != nil {
			t.Logf("Error closing file %s: %v\n", testfilePath, err)
		}
	}()

	testSecurityDescriptors(t, TestFileSDs, testfilePath)
}

func TestSetGetFolderSecurityDescriptors(t *testing.T) {
	tempDir := t.TempDir()
	testfolderPath := filepath.Join(tempDir, "testfolder")
	// create temp folder
	err := os.Mkdir(testfolderPath, os.ModeDir)
	if err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}

	testSecurityDescriptors(t, TestDirSDs, testfolderPath)
}

func testSecurityDescriptors(t *testing.T, testSDs []string, testPath string) {
	for _, testSD := range testSDs {
		sdInputBytes, err := base64.StdEncoding.DecodeString(testSD)
		test.OK(t, errors.Wrapf(err, "Error decoding SD: %s", testPath))

		err = SetSecurityDescriptor(testPath, &sdInputBytes)
		test.OK(t, errors.Wrapf(err, "Error setting file security descriptor for: %s", testPath))

		var sdOutputBytes *[]byte
		sdOutputBytes, err = GetSecurityDescriptor(testPath)
		test.OK(t, errors.Wrapf(err, "Error getting file security descriptor for: %s", testPath))

		CompareSecurityDescriptors(t, testPath, sdInputBytes, *sdOutputBytes)
	}
}
