//go:build !windows
// +build !windows

package restorer

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	restoreui "github.com/restic/restic/internal/ui/restore"
)

func TestRestorerRestoreEmptyHardlinkedFileds(t *testing.T) {
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

	res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		return true, true
	}

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

type printerMock struct {
	s restoreui.State
}

func (p *printerMock) Update(_ restoreui.State, _ time.Duration) {
}
func (p *printerMock) Finish(s restoreui.State, _ time.Duration) {
	p.s = s
}

func TestRestorerProgressBar(t *testing.T) {
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
	res := NewRestorer(repo, sn, Options{Progress: progress})
	res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		return true, true
	}

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
