package cache

import (
	"context"
	"io"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Backend wraps a restic.Backend and adds a cache.
type Backend struct {
	restic.Backend
	restic.Cache
}

// ensure cachedBackend implements restic.Backend
var _ restic.Backend = &Backend{}

// Remove deletes a file from the backend and the cache if it has been cached.
func (b *Backend) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("cache Remove(%v)", h)
	err := b.Backend.Remove(ctx, h)
	if err != nil {
		return err
	}

	return b.Cache.Remove(h)
}

type teeReader struct {
	rd  io.Reader
	wr  io.Writer
	err error
}

func (t *teeReader) Read(p []byte) (n int, err error) {
	n, err = t.rd.Read(p)
	if t.err == nil && n > 0 {
		_, t.err = t.wr.Write(p[:n])
	}

	return n, err
}

var autoCacheTypes = map[restic.FileType]struct{}{
	restic.IndexFile:    struct{}{},
	restic.SnapshotFile: struct{}{},
}

// Save stores a new file is the backend and the cache.
func (b *Backend) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {
	debug.Log("cache Save(%v)", h)
	if _, ok := autoCacheTypes[h.Type]; !ok {
		return b.Backend.Save(ctx, h, rd)
	}

	wr, err := b.Cache.SaveWriter(h)
	if err != nil {
		debug.Log("unable to save object to cache: %v", err)
		return b.Backend.Save(ctx, h, rd)
	}

	tr := &teeReader{rd: rd, wr: wr}
	err = b.Backend.Save(ctx, h, tr)
	if err != nil {
		wr.Close()
		b.Cache.Remove(h)
		return err
	}

	err = wr.Close()
	if err != nil {
		debug.Log("cache writer returned error: %v", err)
		b.Cache.Remove(h)
	}
	return nil
}

var autoCacheFiles = map[restic.FileType]bool{
	restic.IndexFile:    true,
	restic.SnapshotFile: true,
}

// Load loads a file from the cache or the backend.
func (b *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	if b.Cache.Has(h) {
		debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
		return b.Cache.Load(h, length, offset)
	}

	debug.Log("Load(%v, %v, %v) delegated to backend", h, length, offset)
	rd, err := b.Backend.Load(ctx, h, length, offset)
	if err != nil {
		if b.Backend.IsNotExist(err) {
			// try to remove from the cache, ignore errors
			_ = b.Cache.Remove(h)
		}

		return nil, err
	}

	// only cache complete files
	if offset != 0 || length != 0 {
		debug.Log("won't store partial file %v", h)
		return rd, err
	}

	if _, ok := autoCacheFiles[h.Type]; !ok {
		debug.Log("wrong type for auto store %v", h)
		return rd, nil
	}

	debug.Log("auto-store %v in the cache", h)

	// cache the file, then return cached copy
	if err = b.Cache.Save(h, rd); err != nil {
		return nil, err
	}

	if err = rd.Close(); err != nil {
		// try to remove from the cache, ignore errors
		_ = b.Cache.Remove(h)
		return nil, err
	}

	// load from the cache and save in the backend
	return b.Cache.Load(h, 0, 0)
}

// Stat tests whether the backend has a file. If it does not exist but still
// exists in the cache, it is removed from the cache.
func (b *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("cache Stat(%v)", h)

	fi, err := b.Backend.Stat(ctx, h)
	if err != nil {
		if b.Backend.IsNotExist(err) {
			// try to remove from the cache, ignore errors
			_ = b.Cache.Remove(h)
		}

		return fi, err
	}

	return fi, err
}

// IsNotExist returns true if the error is caused by a non-existing file.
func (b *Backend) IsNotExist(err error) bool {
	return b.Backend.IsNotExist(err)
}
