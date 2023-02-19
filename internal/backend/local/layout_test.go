package local

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestLayout(t *testing.T) {
	path := rtest.TempDir(t)

	var tests = []struct {
		filename        string
		layout          string
		failureExpected bool
		packfiles       map[string]bool
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
			be, err := Open(context.TODO(), Config{
				Path:        repo,
				Layout:      test.layout,
				Connections: 2,
			})
			if err != nil {
				t.Fatal(err)
			}

			if be == nil {
				t.Fatalf("Open() returned nil but no error")
			}

			packs := make(map[string]bool)
			err = be.List(context.TODO(), restic.PackFile, func(fi restic.FileInfo) error {
				packs[fi.Name] = false
				return nil
			})

			if err != nil {
				t.Fatalf("List() returned error %v", err)
			}

			if len(packs) == 0 {
				t.Errorf("List() returned zero pack files")
			}

			for id := range test.packfiles {
				if _, ok := packs[id]; !ok {
					t.Errorf("packfile with id %v not found", id)
				}

				packs[id] = true
			}

			for id, v := range packs {
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
