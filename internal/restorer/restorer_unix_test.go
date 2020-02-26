//+build !windows

package restorer

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/fs"
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

func TestRestorerSparseFiles(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	var zeros [1<<20 + 13]byte // 1MB + a bit to get a corner case

	target := &fs.Reader{
		Mode:       0600,
		Name:       "/zeros",
		ReadCloser: ioutil.NopCloser(bytes.NewReader(zeros[:])),
	}
	sc := archiver.NewScanner(target)
	err := sc.Scan(context.TODO(), []string{"/zeros"})
	rtest.OK(t, err)

	arch := archiver.New(repo, target, archiver.Options{})
	_, id, err := arch.Snapshot(context.Background(), []string{"/zeros"},
		archiver.SnapshotOptions{})

	res, err := NewRestorer(repo, id)
	rtest.OK(t, err)

	tempdir, cleanup := rtest.TempDir(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	filename := filepath.Join(tempdir, "zeros")
	content, err := ioutil.ReadFile(filename)
	rtest.OK(t, err)

	rtest.Equals(t, zeros[:], content)

	fi, err := os.Stat(filename)
	rtest.OK(t, err)
	st := fi.Sys().(*syscall.Stat_t)
	if st == nil {
		return
	}

	// This reports 0 blocks on a supporting filesystem.
	// We can't assert that, though, since we don't know to what type
	// of filesystem we're writing.
	t.Logf("wrote %d zeros as %d blocks", len(zeros), st.Blocks)
}

func BenchmarkAllZero(b *testing.B) {
	// Only benchmark the case were the blocks are not all zeros,
	// to ensure that the common case doesn't suffer a performance hit
	// from sparse file support.

	var (
		buf   [4<<20 + 37]byte
		r     = rand.New(rand.NewSource(0x618732))
		zeros int
	)

	b.ReportAllocs()
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		j := r.Intn(len(buf))
		buf[j] = 0xff

		z := allZero(buf[:])
		if z {
			zeros++
		}

		buf[j] = 0
	}

	// Make sure the compiler doesn't optimize away the call to allZeros.
	rtest.Equals(b, zeros, 0)
}
