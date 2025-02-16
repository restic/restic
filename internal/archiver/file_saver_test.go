package archiver

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

func createBigTestFile(t *testing.T) (string, int, []byte) {

	tempdir := test.TempDir(t)

	fileSize := 1024
	fileData := make([]byte, fileSize)

	for i := range fileData {
		fileData[i] = byte(i % 251)
	}

	filename := "file"
	err := os.WriteFile(filepath.Join(tempdir, filename), fileData, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(tempdir, filename)

	return file, fileSize, fileData
}

func startFileSaver(ctx context.Context, t testing.TB, fsInst fs.FS, blockSize uint) (*fileSaver, context.Context, *errgroup.Group, map[restic.ID][]byte) {
	wg, ctx := errgroup.WithContext(ctx)

	savedDataMap := make(map[restic.ID][]byte)

	var lock sync.Mutex

	saveBlob := func(ctx context.Context, tpe restic.BlobType, buf *buffer, _ string, cb func(saveBlobResponse)) {
		id := restic.Hash(buf.Data)
		lock.Lock()
		savedDataMap[id] = buf.Data
		lock.Unlock()
		cb(saveBlobResponse{
			id:         id,
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

	s := newFileSaver(ctx, wg, saveBlob, pol, workers, workers, blockSize)
	s.NodeFromFileInfo = func(snPath, filename string, meta ToNoder, ignoreXattrListError bool) (*restic.Node, error) {
		return meta.ToNode(ignoreXattrListError, false)
	}

	return s, ctx, wg, savedDataMap
}

func TestFileSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	files := createTestFiles(t, 15)

	startFn := func() {}
	completeReadingFn := func() {}
	completeFn := func(*restic.Node, ItemStats) {}

	testFs := fs.Local{}
	s, ctx, wg, _ := startFileSaver(ctx, t, testFs, 0)

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

func TestFileSaverBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// we need a bigger file to account for potential races
	file, fileSize, content := createBigTestFile(t)

	startFn := func() {}
	completeReadingFn := func() {}
	completeFn := func(*restic.Node, ItemStats) {}

	testFs := fs.Local{}
	s, ctx, wg, savedDataMap := startFileSaver(ctx, t, testFs, 8)

	f, err := testFs.OpenFile(file, os.O_RDONLY, false)
	if err != nil {
		t.Fatal(err)
	}
	fn := s.Save(ctx, file, file, f, startFn, completeReadingFn, completeFn)

	fnr := fn.take(ctx)
	if fnr.err != nil {
		t.Errorf("unable to save file: %v", fnr.err)
	}

	s.TriggerShutdown()

	err = wg.Wait()
	if err != nil {
		t.Fatal(err)
	}

	savedContent := make([]byte, 0, fileSize)
	for _, id := range fnr.node.Content {
		savedContent = append(savedContent, savedDataMap[id]...)
	}

	test.Assert(t, bytes.Equal(savedContent, content), "saved content is not identical to the original one")
}
