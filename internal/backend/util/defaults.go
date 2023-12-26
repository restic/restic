package util

import (
	"context"
	"io"

	"github.com/restic/restic/internal/backend"
)

// DefaultLoad implements Backend.Load using lower-level openReader func
func DefaultLoad(ctx context.Context, h backend.Handle, length int, offset int64,
	openReader func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error),
	fn func(rd io.Reader) error) error {

	rd, err := openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(rd)
	if err != nil {
		_ = rd.Close() // ignore secondary errors closing the reader
		return err
	}
	return rd.Close()
}

// DefaultDelete removes all restic keys in the bucket. It will not remove the bucket itself.
func DefaultDelete(ctx context.Context, be backend.Backend) error {
	alltypes := []backend.FileType{
		backend.PackFile,
		backend.KeyFile,
		backend.LockFile,
		backend.SnapshotFile,
		backend.IndexFile}

	for _, t := range alltypes {
		err := be.List(ctx, t, func(fi backend.FileInfo) error {
			return be.Remove(ctx, backend.Handle{Type: t, Name: fi.Name})
		})
		if err != nil {
			return nil
		}
	}
	err := be.Remove(ctx, backend.Handle{Type: backend.ConfigFile})
	if err != nil && be.IsNotExist(err) {
		err = nil
	}

	return err
}
