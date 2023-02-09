package dump

import (
	"bytes"
	"context"
	"testing"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func prepareTempdirRepoSrc(t testing.TB, src archiver.TestDir) (tempdir string, repo restic.Repository, cleanup func()) {
	tempdir, removeTempdir := rtest.TempDir(t)
	repo, removeRepository := repository.TestRepository(t)

	archiver.TestCreateFiles(t, tempdir, src)

	cleanup = func() {
		removeRepository()
		removeTempdir()
	}

	return tempdir, repo, cleanup
}

type CheckDump func(t *testing.T, testDir string, testDump *bytes.Buffer) error

func WriteTest(t *testing.T, format string, cd CheckDump) {
	tests := []struct {
		name   string
		args   archiver.TestDir
		target string
	}{
		{
			name: "single file in root",
			args: archiver.TestDir{
				"file": archiver.TestFile{Content: "string"},
			},
			target: "/",
		},
		{
			name: "multiple files in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestFile{Content: "string"},
			},
			target: "/",
		},
		{
			name: "multiple files and folders in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestFile{Content: "string"},
				"firstDir": archiver.TestDir{
					"another": archiver.TestFile{Content: "string"},
				},
				"secondDir": archiver.TestDir{
					"another2": archiver.TestFile{Content: "string"},
				},
			},
			target: "/",
		},
		{
			name: "file and symlink in root",
			args: archiver.TestDir{
				"file1": archiver.TestFile{Content: "string"},
				"file2": archiver.TestSymlink{Target: "file1"},
			},
			target: "/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tmpdir, repo, cleanup := prepareTempdirRepoSrc(t, tt.args)
			defer cleanup()

			arch := archiver.New(repo, fs.Track{FS: fs.Local{}}, archiver.Options{})

			back := rtest.Chdir(t, tmpdir)
			defer back()

			sn, _, err := arch.Snapshot(ctx, []string{"."}, archiver.SnapshotOptions{})
			rtest.OK(t, err)

			tree, err := restic.LoadTree(ctx, repo, *sn.Tree)
			rtest.OK(t, err)

			dst := &bytes.Buffer{}
			d := New(format, repo, dst)
			if err := d.DumpTree(ctx, tree, tt.target); err != nil {
				t.Fatalf("Dumper.Run error = %v", err)
			}
			if err := cd(t, tmpdir, dst); err != nil {
				t.Errorf("WriteDump() = does not match: %v", err)
			}
		})
	}
}
