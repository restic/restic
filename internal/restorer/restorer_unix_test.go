//+build !windows

package restorer

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestRestorerRestoreEmptyHardlinkedFileds(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	_, id := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file1": File{Links: 2, Inode: 1},
					"file2": File{Links: 2, Inode: 1},
				},
			},
		},
	})

	res, err := NewRestorer(repo, id)
	rtest.OK(t, err)

	res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		return true, true
	}

	tempdir, cleanup := rtest.TempDir(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = res.RestoreTo(ctx, tempdir)
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
