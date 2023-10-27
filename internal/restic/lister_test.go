package restic_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type ListHelper struct {
	ListFn func(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error
}

func (l *ListHelper) List(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error {
	return l.ListFn(ctx, t, fn)
}

func TestMemoizeList(t *testing.T) {
	// setup backend to serve as data source for memoized list
	be := &ListHelper{}

	type FileInfo struct {
		ID   restic.ID
		Size int64
	}
	files := []FileInfo{
		{ID: restic.NewRandomID(), Size: 42},
		{ID: restic.NewRandomID(), Size: 45},
	}
	be.ListFn = func(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error {
		for _, fi := range files {
			if err := fn(fi.ID, fi.Size); err != nil {
				return err
			}
		}
		return nil
	}

	mem, err := restic.MemorizeList(context.TODO(), be, backend.SnapshotFile)
	rtest.OK(t, err)

	err = mem.List(context.TODO(), backend.IndexFile, func(id restic.ID, size int64) error {
		t.Fatal("file type mismatch")
		return nil // the memoized lister must return an error by itself
	})
	rtest.Assert(t, err != nil, "missing error on file typ mismatch")

	var memFiles []FileInfo
	err = mem.List(context.TODO(), backend.SnapshotFile, func(id restic.ID, size int64) error {
		memFiles = append(memFiles, FileInfo{ID: id, Size: size})
		return nil
	})
	rtest.OK(t, err)
	rtest.Equals(t, files, memFiles)
}

func TestMemoizeListError(t *testing.T) {
	// setup backend to serve as data source for memoized list
	be := &ListHelper{}
	be.ListFn = func(ctx context.Context, t backend.FileType, fn func(restic.ID, int64) error) error {
		return fmt.Errorf("list error")
	}
	_, err := restic.MemorizeList(context.TODO(), be, backend.SnapshotFile)
	rtest.Assert(t, err != nil, "missing error on list error")
}
