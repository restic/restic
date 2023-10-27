package logger

import (
	"context"
	"io"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
)

type Backend struct {
	backend.Backend
}

// statically ensure that Backend implements backend.Backend.
var _ backend.Backend = &Backend{}

func New(be backend.Backend) *Backend {
	return &Backend{Backend: be}
}

func (be *Backend) IsNotExist(err error) bool {
	isNotExist := be.Backend.IsNotExist(err)
	debug.Log("IsNotExist(%T, %#v, %v)", err, err, isNotExist)
	return isNotExist
}

// Save adds new Data to the backend.
func (be *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	debug.Log("Save(%v, %v)", h, rd.Length())
	err := be.Backend.Save(ctx, h, rd)
	debug.Log("  save err %v", err)
	return err
}

// Remove deletes a file from the backend.
func (be *Backend) Remove(ctx context.Context, h backend.Handle) error {
	debug.Log("Remove(%v)", h)
	err := be.Backend.Remove(ctx, h)
	debug.Log("  remove err %v", err)
	return err
}

func (be *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(io.Reader) error) error {
	debug.Log("Load(%v, length %v, offset %v)", h, length, offset)
	err := be.Backend.Load(ctx, h, length, offset, fn)
	debug.Log("  load err %v", err)
	return err
}

func (be *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	debug.Log("Stat(%v)", h)
	fi, err := be.Backend.Stat(ctx, h)
	debug.Log("  stat err %v", err)
	return fi, err
}

func (be *Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	debug.Log("List(%v)", t)
	err := be.Backend.List(ctx, t, fn)
	debug.Log("  list err %v", err)
	return err
}

func (be *Backend) Delete(ctx context.Context) error {
	debug.Log("Delete()")
	err := be.Backend.Delete(ctx)
	debug.Log("  delete err %v", err)
	return err
}

func (be *Backend) Close() error {
	debug.Log("Close()")
	err := be.Backend.Close()
	debug.Log("  close err %v", err)
	return err
}

func (be *Backend) Unwrap() backend.Backend { return be.Backend }
