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
	})

	res := NewRestorer(repo, sn, false, nil)

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
	filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64
}

func (p *printerMock) Update(_, _, _, _ uint64, _ time.Duration) {
}
func (p *printerMock) Finish(filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64, _ time.Duration) {
	p.filesFinished = filesFinished
	p.filesTotal = filesTotal
	p.allBytesWritten = allBytesWritten
	p.allBytesTotal = allBytesTotal
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
	})

	mock := &printerMock{}
	progress := restoreui.NewProgress(mock, 0)
	res := NewRestorer(repo, sn, false, progress)
	res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		return true, true
	}

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)
	progress.Finish()

	const filesFinished = 4
	const filesTotal = filesFinished
	const allBytesWritten = 10
	const allBytesTotal = allBytesWritten
	rtest.Assert(t, mock.filesFinished == filesFinished, "filesFinished: expected %v, got %v", filesFinished, mock.filesFinished)
	rtest.Assert(t, mock.filesTotal == filesTotal, "filesTotal: expected %v, got %v", filesTotal, mock.filesTotal)
	rtest.Assert(t, mock.allBytesWritten == allBytesWritten, "allBytesWritten: expected %v, got %v", allBytesWritten, mock.allBytesWritten)
	rtest.Assert(t, mock.allBytesTotal == allBytesTotal, "allBytesTotal: expected %v, got %v", allBytesTotal, mock.allBytesTotal)
}
