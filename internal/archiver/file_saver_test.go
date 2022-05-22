package archiver

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

func createTestFiles(t testing.TB, num int) (files []string, cleanup func()) {
	tempdir, cleanup := test.TempDir(t)

	for i := 0; i < 15; i++ {
		filename := fmt.Sprintf("testfile-%d", i)
		err := ioutil.WriteFile(filepath.Join(tempdir, filename), []byte(filename), 0600)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, filepath.Join(tempdir, filename))
	}

	return files, cleanup
}

func startFileSaver(ctx context.Context, t testing.TB) (*FileSaver, context.Context, *errgroup.Group) {
	wg, ctx := errgroup.WithContext(ctx)

	saveBlob := func(ctx context.Context, tpe restic.BlobType, buf *Buffer) FutureBlob {
		ch := make(chan SaveBlobResponse)
		close(ch)
		return FutureBlob{ch: ch}
	}

	workers := uint(runtime.NumCPU())
	pol, err := chunker.RandomPolynomial()
	if err != nil {
		t.Fatal(err)
	}

	s := NewFileSaver(ctx, wg, saveBlob, pol, workers, workers)
	s.NodeFromFileInfo = restic.NodeFromFileInfo

	return s, ctx, wg
}

func TestFileSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	files, cleanup := createTestFiles(t, 15)
	defer cleanup()

	startFn := func() {}
	completeFn := func(*restic.Node, ItemStats) {}

	testFs := fs.Local{}
	s, ctx, wg := startFileSaver(ctx, t)

	var results []FutureNode

	for _, filename := range files {
		f, err := testFs.Open(filename)
		if err != nil {
			t.Fatal(err)
		}

		fi, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}

		ff := s.Save(ctx, filename, filename, f, fi, startFn, completeFn)
		results = append(results, ff)
	}

	for _, file := range results {
		fnr := file.take(ctx)
		if fnr.err != nil {
			t.Errorf("unable to save file: %v", fnr.err)
		}
	}

	s.TriggerShutdown()

	err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
