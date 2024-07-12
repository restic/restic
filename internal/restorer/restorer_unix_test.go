//go:build !windows
// +build !windows

package restorer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
	restoreui "github.com/restic/restic/internal/ui/restore"
)

func TestRestorerRestoreEmptyHardlinkedFields(t *testing.T) {
	repo := repository.TestRepository(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file1": File{Links: 2, Inode: 1},
					"file2": File{Links: 2, Inode: 1},
				},
			},
		},
	}, noopGetGenericAttributes)

	res := NewRestorer(repo, sn, Options{})

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	f1, err := os.Stat(filepath.Join(tempdir, "dirtest/file1"))
	rtest.OK(t, err)
	rtest.Equals(t, int64(0), f1.Size())
	s1, ok1 := f1.Sys().(*syscall.Stat_t)

	f2, err := os.Stat(filepath.Join(tempdir, "dirtest/file2"))
	rtest.OK(t, err)
	rtest.Equals(t, int64(0), f2.Size())
	s2, ok2 := f2.Sys().(*syscall.Stat_t)

	if ok1 && ok2 {
		rtest.Equals(t, s1.Ino, s2.Ino)
	}
}

func getBlockCount(t *testing.T, filename string) int64 {
	fi, err := os.Stat(filename)
	rtest.OK(t, err)
	st := fi.Sys().(*syscall.Stat_t)
	if st == nil {
		return -1
	}
	return st.Blocks
}

func TestRestorerProgressBar(t *testing.T) {
	testRestorerProgressBar(t, false)
}

func TestRestorerProgressBarDryRun(t *testing.T) {
	testRestorerProgressBar(t, true)
}

func testRestorerProgressBar(t *testing.T, dryRun bool) {
	repo := repository.TestRepository(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file1": File{Links: 2, Inode: 1, Data: "foo"},
					"file2": File{Links: 2, Inode: 1, Data: "foo"},
				},
			},
			"file2": File{Links: 1, Inode: 2, Data: "example"},
		},
	}, noopGetGenericAttributes)

	mock := &printerMock{}
	progress := restoreui.NewProgress(mock, 0)
	res := NewRestorer(repo, sn, Options{Progress: progress, DryRun: dryRun})

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)
	progress.Finish()

	rtest.Equals(t, restoreui.State{
		FilesFinished:   4,
		FilesTotal:      4,
		FilesSkipped:    0,
		AllBytesWritten: 10,
		AllBytesTotal:   10,
		AllBytesSkipped: 0,
	}, mock.s)
}

func TestRestorePermissions(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n", Mode: 0o600, ModTime: time.Now()},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sn, id := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	t.Logf("snapshot saved as %v", id.Str())

	res := NewRestorer(repo, sn, Options{})
	rtest.OK(t, res.RestoreTo(ctx, tempdir))

	for _, overwrite := range []OverwriteBehavior{OverwriteIfChanged, OverwriteAlways} {
		// tamper with permissions
		path := filepath.Join(tempdir, "foo")
		rtest.OK(t, os.Chmod(path, 0o700))

		res = NewRestorer(repo, sn, Options{Overwrite: overwrite})
		rtest.OK(t, res.RestoreTo(ctx, tempdir))
		fi, err := os.Stat(path)
		rtest.OK(t, err)
		rtest.Equals(t, fs.FileMode(0o600), fi.Mode().Perm(), "unexpected permissions")
	}
}
