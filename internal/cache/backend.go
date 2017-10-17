package cache

import (
	"context"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Backend wraps a restic.Backend and adds a cache.
type Backend struct {
	restic.Backend
	*Cache

	// inProgress contains the handle for all files that are currently
	// downloaded. The channel in the value is closed as soon as the download
	// is finished.
	inProgressMutex sync.Mutex
	inProgress      map[restic.Handle]chan struct{}
}

// ensure cachedBackend implements restic.Backend
var _ restic.Backend = &Backend{}

func newBackend(be restic.Backend, c *Cache) *Backend {
	return &Backend{
		Backend:    be,
		Cache:      c,
		inProgress: make(map[restic.Handle]chan struct{}),
	}
}

// Remove deletes a file from the backend and the cache if it has been cached.
func (b *Backend) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("cache Remove(%v)", h)
	err := b.Backend.Remove(ctx, h)
	if err != nil {
		return err
	}

	return b.Cache.Remove(h)
}

var autoCacheTypes = map[restic.FileType]struct{}{
	restic.IndexFile:    struct{}{},
	restic.SnapshotFile: struct{}{},
}

// Save stores a new file in the backend and the cache.
func (b *Backend) Save(ctx context.Context, h restic.Handle, rd io.Reader) (err error) {
	if _, ok := autoCacheTypes[h.Type]; !ok {
		return b.Backend.Save(ctx, h, rd)
	}

	debug.Log("Save(%v): auto-store in the cache", h)

	seeker, ok := rd.(io.Seeker)
	if !ok {
		return errors.New("reader is not a seeker")
	}

	pos, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrapf(err, "Seek")
	}

	if pos != 0 {
		return errors.Errorf("reader is not rewind (pos %d)", pos)
	}

	err = b.Backend.Save(ctx, h, rd)
	if err != nil {
		return err
	}

	_, err = seeker.Seek(pos, io.SeekStart)
	if err != nil {
		return errors.Wrapf(err, "Seek")
	}

	err = b.Cache.Save(h, rd)
	if err != nil {
		debug.Log("unable to save %v to cache: %v", h, err)
		_ = b.Cache.Remove(h)
		return nil
	}

	return nil
}

var autoCacheFiles = map[restic.FileType]bool{
	restic.IndexFile:    true,
	restic.SnapshotFile: true,
}

func (b *Backend) cacheFile(ctx context.Context, h restic.Handle) error {
	finish := make(chan struct{})
	defer func() {
		close(finish)

		// remove the finish channel from the map
		b.inProgressMutex.Lock()
		delete(b.inProgress, h)
		b.inProgressMutex.Unlock()
	}()

	b.inProgressMutex.Lock()
	other, alreadyDownloading := b.inProgress[h]
	if !alreadyDownloading {
		b.inProgress[h] = finish
	}
	b.inProgressMutex.Unlock()

	if alreadyDownloading {
		debug.Log("readahead %v is already performed by somebody else, delegating...", h)
		<-other
		debug.Log("download %v finished", h)
		return nil
	}

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

// loadFromCacheOrDelegate will try to load the file from the cache, and fall
// back to the backend if that fails.
func (b *Backend) loadFromCacheOrDelegate(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	rd, err := b.Cache.Load(h, length, offset)
	if err == nil {
		return rd, nil
	}

	return b.Backend.Load(ctx, h, length, offset)
}

// Load loads a file from the cache or the backend.
func (b *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	b.inProgressMutex.Lock()
	waitForFinish, inProgress := b.inProgress[h]
	b.inProgressMutex.Unlock()

	if inProgress {
		debug.Log("downloading %v is already in progress, waiting for finish", h)
		<-waitForFinish
		debug.Log("downloading %v finished", h)
	}

	if b.Cache.Has(h) {
		debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
		rd, err := b.Cache.Load(h, length, offset)
		if err == nil {
			return rd, nil
		}
		debug.Log("error loading %v from cache: %v", h, err)
	}

	// partial file requested
	if offset != 0 || length != 0 {
		if b.Cache.PerformReadahead(h) {
			debug.Log("performing readahead for %v", h)

			err := b.cacheFile(ctx, h)
			if err == nil {
				return b.loadFromCacheOrDelegate(ctx, h, length, offset)
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
