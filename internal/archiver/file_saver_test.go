package archiver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

func createTestFiles(t testing.TB, num int) (files []string) {
	tempdir := test.TempDir(t)

	for i := 0; i < num; i++ {
		filename := fmt.Sprintf("testfile-%d", i)
		err := os.WriteFile(filepath.Join(tempdir, filename), []byte(filename), 0600)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, filepath.Join(tempdir, filename))
	}

	return files
}

func startFileSaver(ctx context.Context, t testing.TB, fsInst fs.FS) (*fileSaver, context.Context, *errgroup.Group) {
	wg, ctx := errgroup.WithContext(ctx)

	saveBlob := func(ctx context.Context, tpe restic.BlobType, buf *buffer, _ string, cb func(saveBlobResponse)) {
		cb(saveBlobResponse{
			id:         restic.Hash(buf.Data),
			length:     len(buf.Data),
			sizeInRepo: len(buf.Data),
			known:      false,
		})
	}

	workers := uint(runtime.NumCPU())
	pol, err := chunker.RandomPolynomial()
	if err != nil {
		t.Fatal(err)
	}

	s := newFileSaver(ctx, wg, saveBlob, pol, workers, workers)
	s.NodeFromFileInfo = func(snPath, filename string, meta ToNoder, ignoreXattrListError bool) (*restic.Node, error) {
		return meta.ToNode(ignoreXattrListError)
	}

	return s, ctx, wg
}

func TestFileSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	files := createTestFiles(t, 15)

	startFn := func() {}
	completeReadingFn := func() {}
	completeFn := func(*restic.Node, ItemStats) {}

	testFs := fs.Local{}
	s, ctx, wg := startFileSaver(ctx, t, testFs)

	var results []futureNode

	for _, filename := range files {
		f, err := testFs.OpenFile(filename, os.O_RDONLY, false)
		if err != nil {
			t.Fatal(err)
		}

		ff := s.Save(ctx, filename, filename, f, startFn, completeReadingFn, completeFn)
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
