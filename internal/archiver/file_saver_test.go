package archiver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/frontend"
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

func startFileSaver(ctx context.Context, t testing.TB) (*FileSaver, context.Context, *errgroup.Group) {
	wg, ctx := errgroup.WithContext(ctx)

	saveBlob := func(ctx context.Context, tpe restic.BlobType, buf *Buffer, _ string, cb func(SaveBlobResponse)) {
		cb(SaveBlobResponse{
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

	s := NewFileSaver(ctx, wg, saveBlob, pol, workers, workers)
	return s, ctx, wg
}

func TestFileSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	files := createTestFiles(t, 15)

	startFn := func() {}
	completeReadingFn := func() {}
	completeFn := func(*restic.Node, ItemStats) {}

	testFrontend := frontend.LocalFrontend{FS: fs.Local{}}
	s, ctx, wg := startFileSaver(ctx, t)

	var results []FutureNode

	for _, filename := range files {
		fm := testFrontend.Prepare(filename)[0]
		err := fm.Init()
		if err != nil {
			t.Fatal(err)
		}

		fc, err := fm.OpenFile()
		if err != nil {
			t.Fatal(err)
		}

		ff := s.Save(ctx, filename, fm, fc, startFn, completeReadingFn, completeFn)
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
