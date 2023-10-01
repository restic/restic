package util

import (
	"context"
	"io"

	"github.com/restic/restic/internal/restic"
)

// DefaultLoad implements Backend.Load using lower-level openReader func
func DefaultLoad(ctx context.Context, h restic.Handle, length int, offset int64,
	openReader func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error),
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
func DefaultDelete(ctx context.Context, be restic.Backend) error {
	alltypes := []restic.FileType{
		restic.PackFile,
		restic.KeyFile,
		restic.LockFile,
		restic.SnapshotFile,
		restic.IndexFile}

	for _, t := range alltypes {
		err := be.List(ctx, t, func(fi restic.FileInfo) error {
			return be.Remove(ctx, restic.Handle{Type: t, Name: fi.Name})
		})
		if err != nil {
			return nil
		}
	}
	err := be.Remove(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil && be.IsNotExist(err) {
		err = nil
	}

	return err
}
