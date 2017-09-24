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
	*Cache
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
	if _, ok := autoCacheTypes[h.Type]; !ok {
		return b.Backend.Save(ctx, h, rd)
	}

	debug.Log("Save(%v): auto-store in the cache", h)
	wr, err := b.Cache.SaveWriter(h)
	if err != nil {
		debug.Log("unable to save %v to cache: %v", h, err)
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
		_ = b.Cache.Remove(h)
	}
	return nil
}

var autoCacheFiles = map[restic.FileType]bool{
	restic.IndexFile:    true,
	restic.SnapshotFile: true,
}

func (b *Backend) cacheFile(ctx context.Context, h restic.Handle) error {
	rd, err := b.Backend.Load(ctx, h, 0, 0)
	if err != nil {
		return err
	}

	if err = b.Cache.Save(h, rd); err != nil {
		return err
	}

	if err = rd.Close(); err != nil {
		// try to remove from the cache, ignore errors
		_ = b.Cache.Remove(h)
		return err
	}

	return nil
}

// Load loads a file from the cache or the backend.
func (b *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	if b.Cache.Has(h) {
		debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
		return b.Cache.Load(h, length, offset)
	}

	// partial file requested
	if offset != 0 || length != 0 {
		if b.Cache.PerformReadahead(h) {
			debug.Log("performing readahead for %v", h)
			err := b.cacheFile(ctx, h)
			if err == nil {
				return b.Cache.Load(h, length, offset)
			}

			debug.Log("error caching %v: %v", h, err)
		}

		debug.Log("Load(%v, %v, %v): partial file requested, delegating to backend", h, length, offset)
		return b.Backend.Load(ctx, h, length, offset)
	}

	// if we don't automatically cache this file type, fall back to the backend
	if _, ok := autoCacheFiles[h.Type]; !ok {
		debug.Log("Load(%v, %v, %v): delegating to backend", h, length, offset)
		return b.Backend.Load(ctx, h, length, offset)
	}

	debug.Log("auto-store %v in the cache", h)
	err := b.cacheFile(ctx, h)

	if err == nil {
		// load the cached version
		return b.Cache.Load(h, 0, 0)
	}

	debug.Log("error caching %v: %v, falling back to backend", h, err)
	return b.Backend.Load(ctx, h, length, offset)
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
