package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Backend wraps a restic.Backend and adds a cache.
type Backend struct {
	backend.Backend
	*Cache

	// inProgress contains the handle for all files that are currently
	// downloaded. The channel in the value is closed as soon as the download
	// is finished.
	inProgressMutex sync.Mutex
	inProgress      map[backend.Handle]chan struct{}
	errorLog        func(string, ...interface{})
}

// ensure Backend implements backend.Backend
var _ backend.Backend = &Backend{}

func newBackend(be backend.Backend, c *Cache, errorLog func(string, ...interface{})) *Backend {
	return &Backend{
		Backend:    be,
		Cache:      c,
		inProgress: make(map[backend.Handle]chan struct{}),
		errorLog:   errorLog,
	}
}

// Remove deletes a file from the backend and the cache if it has been cached.
func (b *Backend) Remove(ctx context.Context, h backend.Handle) error {
	err := b.Backend.Remove(ctx, h)
	if err != nil {
		return err
	}

	_, err = b.Cache.remove(h)
	return err
}

func autoCacheTypes(h backend.Handle) bool {
	switch h.Type {
	case backend.IndexFile, backend.SnapshotFile:
		return true
	case backend.PackFile:
		return h.IsMetadata
	}
	return false
}

// Save stores a new file in the backend and the cache.
func (b *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if !autoCacheTypes(h) {
		return b.Backend.Save(ctx, h, rd)
	}

	debug.Log("Save(%v): auto-store in the cache", h)

	// make sure the reader is at the start
	err := rd.Rewind()
	if err != nil {
		return err
	}

	// first, save in the backend
	err = b.Backend.Save(ctx, h, rd)
	if err != nil {
		return err
	}

	// next, save in the cache
	err = rd.Rewind()
	if err != nil {
		return err
	}

	err = b.Cache.save(h, rd)
	if err != nil {
		debug.Log("unable to save %v to cache: %v", h, err)
		return err
	}

	return nil
}

func (b *Backend) tryToCacheFile(ctx context.Context, h backend.Handle) error {
	finish := make(chan struct{})

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

	defer func() {
		// signal other waiting goroutines that the file may now be cached
		close(finish)

		// remove the finish channel from the map
		b.inProgressMutex.Lock()
		delete(b.inProgress, h)
		b.inProgressMutex.Unlock()
	}()

	if b.hasToVerify(h) && b.Cache.Has(h) {
		if err := b.verify(ctx, h); err != nil {
			return err
		}
	}

	// test again, maybe the file was cached in the meantime
	if !b.Cache.Has(h) {
		// nope, it's still not in the cache, pull it from the repo and save it
		err := b.Backend.Load(ctx, h, 0, 0, func(rd io.Reader) error {
			return b.Cache.save(h, rd)
		})
		if err != nil {
			// try to remove from the cache, ignore errors
			_, _ = b.Cache.remove(h)
		}
		return err
	}

	b.markVerified(h)
	debug.Log("verified %v", h)
	return nil
}

// loadFromCacheOrDelegate will try to load the file from the cache, and fall
// back to the backend if that fails.
func (b *Backend) loadFromCacheOrDelegate(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	rd, _, err := b.Cache.load(h, length, offset)
	if err != nil {
		debug.Log("error caching %v: %v, falling back to backend", h, err)
		return b.Backend.Load(ctx, h, length, offset, consumer)
	}

	err = consumer(rd)
	if err != nil {
		_ = rd.Close() // ignore secondary errors
		return err
	}
	return rd.Close()
}

func (b *Backend) hasToVerify(h backend.Handle) bool {
	if b.Cache.verifiedFiles == nil {
		return false
	}

	b.Cache.verifiedFilesLock.Lock()
	_, ok := b.Cache.verifiedFiles[h]
	b.Cache.verifiedFilesLock.Unlock()
	return !ok
}

func (b *Backend) markVerified(h backend.Handle) {
	if b.Cache.verifiedFiles != nil {
		b.Cache.verifiedFilesLock.Lock()
		b.Cache.verifiedFiles[h] = struct{}{}
		b.Cache.verifiedFilesLock.Unlock()
	}
}

func (b *Backend) verify(ctx context.Context, h backend.Handle) error {
	// verify that the cache file is correct or at least not more broken than the version stored at the backend
	var remoteHash, localHash restic.ID

	err := b.Backend.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		hash := sha256.New()
		_, ierr := io.Copy(hash, rd)
		remoteHash = restic.IDFromHash(hash.Sum(nil))
		return ierr
	})
	if err != nil {
		return err
	}
	err = b.Backend.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		hash := sha256.New()
		_, ierr := io.Copy(hash, rd)
		localHash = restic.IDFromHash(hash.Sum(nil))
		return ierr
	})
	if err != nil {
		return err
	}

	if remoteHash != localHash {
		if remoteHash.String() == h.Name {
			// the remote version is correct, but not the local version
			// delete the local version to repair the cache
			_, _ = b.Cache.remove(h)
		} else if localHash.String() == h.Name {
			return fmt.Errorf("%v: remote file damaged, please re-upload the cached copy", h)
		} else {
			return fmt.Errorf("%v: cached and remote file differ and are both invalid", h)
		}
	}
	return nil
}

// Load loads a file from the cache or the backend.
func (b *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	b.inProgressMutex.Lock()
	waitForFinish, inProgress := b.inProgress[h]
	b.inProgressMutex.Unlock()

	if inProgress {
		debug.Log("downloading %v is already in progress, waiting for finish", h)
		<-waitForFinish
		debug.Log("downloading %v finished", h)
	}

	if b.Cache.Has(h) && !b.hasToVerify(h) {
		debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
		rd, _, err := b.Cache.load(h, length, offset)
		if err == nil {
			err = consumer(rd)
			if err != nil {
				_ = rd.Close() // ignore secondary errors
				return err
			}
			return rd.Close()
		}
		debug.Log("error loading %v from cache: %v", h, err)
	}

	// if we don't automatically cache this file type, fall back to the backend
	if !autoCacheTypes(h) {
		debug.Log("Load(%v, %v, %v): delegating to backend", h, length, offset)
		return b.Backend.Load(ctx, h, length, offset, consumer)
	}

	debug.Log("auto-store %v in the cache", h)
	err := b.tryToCacheFile(ctx, h)
	if err != nil {
		return err
	}
	return b.loadFromCacheOrDelegate(ctx, h, length, offset, consumer)
}

// Stat tests whether the backend has a file. If it does not exist but still
// exists in the cache, it is removed from the cache.
func (b *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	fi, err := b.Backend.Stat(ctx, h)
	if err != nil && b.Backend.IsNotExist(err) {
		// try to remove from the cache, ignore errors
		_, _ = b.Cache.remove(h)
	}

	return fi, err
}

// IsNotExist returns true if the error is caused by a non-existing file.
func (b *Backend) IsNotExist(err error) bool {
	return b.Backend.IsNotExist(err)
}

func (b *Backend) Unwrap() backend.Backend {
	return b.Backend
}

func (b *Backend) List(ctx context.Context, t backend.FileType, fn func(f backend.FileInfo) error) error {
	if !b.Cache.canBeCached(t) {
		return b.Backend.List(ctx, t, fn)
	}

	// will contain the IDs of the files that are in the repository
	ids := restic.NewIDSet()

	// wrap the original function to also add the file to the ids set
	wrapFn := func(f backend.FileInfo) error {
		id, err := restic.ParseID(f.Name)
		if err != nil {
			// ignore files with invalid name
			return nil
		}

		ids.Insert(id)

		// execute the original function
		return fn(f)
	}

	err := b.Backend.List(ctx, t, wrapFn)
	if err != nil {
		return err
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// clear the cache for files that are not in the repo anymore, ignore errors
	err = b.Cache.Clear(t, ids)
	if err != nil {
		b.errorLog("error clearing %s files in cache: %v\n", t.String(), err)
	}

	return nil
}

// Warmup delegates to wrapped backend.
func (b *Backend) Warmup(ctx context.Context, h []backend.Handle) ([]backend.Handle, error) {
	return b.Backend.Warmup(ctx, h)
}

// WarmupWait delegates to wrapped backend.
func (b *Backend) WarmupWait(ctx context.Context, h []backend.Handle) error {
	return b.Backend.WarmupWait(ctx, h)
}
