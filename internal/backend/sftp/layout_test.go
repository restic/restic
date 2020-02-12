package sftp_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestLayout(t *testing.T) {
	if sftpServer == "" {
		t.Skip("sftp server binary not available")
	}

	path, cleanup := rtest.TempDir(t)
	defer cleanup()

	var tests = []struct {
		filename        string
		layout          string
		failureExpected bool
		datafiles       map[string]bool
	}{
		{"repo-layout-default.tar.gz", "", false, map[string]bool{
			"aa464e9fd598fe4202492ee317ffa728e82fa83a1de1a61996e5bd2d6651646c": false,
			"fc919a3b421850f6fa66ad22ebcf91e433e79ffef25becf8aef7c7b1eca91683": false,
			"c089d62788da14f8b7cbf77188305c0874906f0b73d3fce5a8869050e8d0c0e1": false,
		}},
		{"repo-layout-s3legacy.tar.gz", "", false, map[string]bool{
			"fc919a3b421850f6fa66ad22ebcf91e433e79ffef25becf8aef7c7b1eca91683": false,
			"c089d62788da14f8b7cbf77188305c0874906f0b73d3fce5a8869050e8d0c0e1": false,
			"aa464e9fd598fe4202492ee317ffa728e82fa83a1de1a61996e5bd2d6651646c": false,
		}},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			rtest.SetupTarTestFixture(t, path, filepath.Join("..", "testdata", test.filename))

			repo := filepath.Join(path, "repo")
			be, err := sftp.Open(sftp.Config{
				Command: fmt.Sprintf("%q -e", sftpServer),
				Path:    repo,
				Layout:  test.layout,
			})
			if err != nil {
				t.Fatal(err)
			}

			if be == nil {
				t.Fatalf("Open() returned nil but no error")
			}

			datafiles := make(map[string]bool)
			err = be.List(context.TODO(), restic.DataFile, func(fi restic.FileInfo) error {
				datafiles[fi.Name] = false
				return nil
			})
			rtest.OK(t, err)

			if len(datafiles) == 0 {
				t.Errorf("List() returned zero data files")
			}

			for id := range test.datafiles {
				if _, ok := datafiles[id]; !ok {
					t.Errorf("datafile with id %v not found", id)
				}

				datafiles[id] = true
			}

			for id, v := range datafiles {
				if !v {
					t.Errorf("unexpected id %v found", id)
				}
			}

			if err = be.Close(); err != nil {
				t.Errorf("Close() returned error %v", err)
			}

			rtest.RemoveAll(t, filepath.Join(path, "repo"))
		})
	}
}
