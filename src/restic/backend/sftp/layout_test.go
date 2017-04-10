package sftp_test

import (
	"fmt"
	"path/filepath"
	"restic/backend/sftp"
	. "restic/test"
	"testing"
)

func TestLayout(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		filename        string
		layout          string
		failureExpected bool
	}{
		{"repo-layout-local.tar.gz", "", false},
		{"repo-layout-cloud.tar.gz", "", false},
		{"repo-layout-s3-old.tar.gz", "", false},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			SetupTarTestFixture(t, path, filepath.Join("..", "testdata", test.filename))

			repo := filepath.Join(path, "repo")
			be, err := sftp.Open(sftp.Config{
				Command: fmt.Sprintf("%q -e", sftpserver),
				Path:    repo,
				Layout:  test.layout,
			})
			if err != nil {
				t.Fatal(err)
			}

			if be == nil {
				t.Fatalf("Open() returned nil but no error")
			}

			RemoveAll(t, filepath.Join(path, "repo"))
		})
	}
}
