package local

import (
	"path/filepath"
	. "restic/test"
	"testing"
)

func TestLocalLayout(t *testing.T) {
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
			be, err := Open(Config{
				Path:   repo,
				Layout: test.layout,
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
