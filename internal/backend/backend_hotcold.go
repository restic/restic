package backend

import (
	"context"
	"fmt"
	"io"

	"github.com/restic/restic/internal/restic"
)

// HotColdBackend uses a hot and a cold repository
type HotColdBackend struct {
	hot  restic.Backend
	cold restic.Backend
}

// statically ensure that HotColdBackend implements restic.Backend.
var _ restic.Backend = &HotColdBackend{}

func NewHotColdBackend(hot restic.Backend, cold restic.Backend) *HotColdBackend {
	return &HotColdBackend{
		hot:  hot,
		cold: cold,
	}
}

// isCold returns whether the handle is cold or not
func (be *HotColdBackend) isCold(h restic.Handle) bool {
	return (h.Type == restic.PackFile && h.BT == restic.DataBlob)
}

// Save stores the data in the cold backend under the given handle.
// If the handle is hot, it is additonally saved in the hot backend.
func (be *HotColdBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := be.cold.Save(ctx, h, rd); err != nil || be.isCold(h) {
		return err
	}
	if err := rd.Rewind(); err != nil {
		return err
	}
	return be.hot.Save(ctx, h, rd)
}

func (be *HotColdBackend) hasHot(ctx context.Context, h restic.Handle) (bool, error) {
	if h.Type != restic.PackFile {
		return true, nil
	}
	return be.hot.Test(ctx, h)
}

// Load tries to loads data from the hot backend and falls back to the cold backend for pack files.
func (be *HotColdBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, consumer func(rd io.Reader) error) (err error) {
	hasHot, err := be.hasHot(ctx, h)
	if err != nil {
		return err
	}

	if hasHot {
		return be.hot.Load(ctx, h, length, offset, consumer)
	}

	return be.cold.Load(ctx, h, length, offset, consumer)
}

// Stat returns information about the File identified by h.
func (be *HotColdBackend) Stat(ctx context.Context, h restic.Handle) (fi restic.FileInfo, err error) {
	return be.cold.Stat(ctx, h)
}

// Remove removes a File with type t and name.
func (be *HotColdBackend) Remove(ctx context.Context, h restic.Handle) (err error) {
	hasHot, err := be.hasHot(ctx, h)
	if err != nil {
		return err
	}

	if !hasHot {
		return be.cold.Remove(ctx, h)
	}

	errhot := be.hot.Remove(ctx, h)
	errcold := be.cold.Remove(ctx, h)
	if errhot == nil {
		return errcold
	}
	return errhot
}

// Test returns a boolean value whether a File with the name and type exists.
func (be *HotColdBackend) Test(ctx context.Context, h restic.Handle) (exists bool, err error) {
	return be.cold.Test(ctx, h)
}

// List runs fn for each file in the backend which has the type t. When an
// error is returned by the underlying backend, the request is retried. When fn
// returns an error, the operation is aborted and the error is returned to the
// caller.
func (be *HotColdBackend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	return be.cold.List(ctx, t, fn)
}

func (be *HotColdBackend) Close() error {
	errhot := be.hot.Close()
	errcold := be.cold.Close()
	if errhot == nil {
		return errcold
	}
	return errhot
}

func (be *HotColdBackend) Delete(ctx context.Context) error {
	errhot := be.hot.Delete(ctx)
	errcold := be.cold.Delete(ctx)
	if errhot == nil {
		return errcold
	}
	return errhot
}

func (be *HotColdBackend) IsNotExist(err error) bool {
	return be.cold.IsNotExist(err)
}

func (be *HotColdBackend) Location() string {
	return fmt.Sprintf("hot: %v cold: %v", be.hot.Location(), be.cold.Location())
}

func CheckSameFiles(ctx context.Context, be1 restic.Backend, be2 restic.Backend, t restic.FileType) (bool, error) {
	files1 := make(map[string]int64)
	err := be1.List(ctx, t, func(fi restic.FileInfo) error {
		files1[fi.Name] = fi.Size
		return nil
	})
	if err != nil {
		return false, err
	}

	files2 := make(map[string]int64)
	err = be2.List(ctx, t, func(fi restic.FileInfo) error {
		files2[fi.Name] = fi.Size
		return nil
	})
	if err != nil {
		return false, err
	}

	if len(files1) != len(files2) {
		return false, nil
	}

	for file, size := range files1 {
		size2, ok := files2[file]
		if !ok || size2 != size {
			return false, nil
		}
	}
	return true, nil
}
