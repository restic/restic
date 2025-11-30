package restorer

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/cespare/xxhash/v2"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

// writes blobs to target files.
// multiple files can be written to concurrently.
// multiple blobs can be concurrently written to the same file.
// TODO I am not 100% convinced this is necessary, i.e. it may be okay
// to use multiple os.File to write to the same target file
type filesWriter struct {
	buckets              []filesWriterBucket
	allowRecursiveDelete bool
	cacheMu              sync.Mutex
	cache                *simplelru.LRU[string, *partialFile]
}

type filesWriterBucket struct {
	lock  sync.Mutex
	files map[string]*partialFile
}

type partialFile struct {
	*os.File
	users  int // Reference count.
	sparse bool
}

func newFilesWriter(count int, allowRecursiveDelete bool) *filesWriter {
	// use a large number of buckets to minimize bucket contention
	// creating a new file can be slow, so make sure that files typically end up in different buckets.
	buckets := make([]filesWriterBucket, 1024)
	for b := 0; b < len(buckets); b++ {
		buckets[b].files = make(map[string]*partialFile)
	}

	cache, err := simplelru.NewLRU[string, *partialFile](count+50, func(_ string, wr *partialFile) {
		// close the file only when it is not in use
		if wr.users == 0 {
			_ = wr.Close()
		}
	})
	if err != nil {
		panic(err) // can't happen
	}

	return &filesWriter{
		buckets:              buckets,
		allowRecursiveDelete: allowRecursiveDelete,
		cache:                cache,
	}
}

func openFile(path string) (*os.File, error) {
	f, err := fs.OpenFile(path, fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("unexpected file type %v at %q", fi.Mode().Type(), path)
	}
	return f, nil
}

func createFile(path string, createSize int64, sparse bool, allowRecursiveDelete bool) (*os.File, error) {
	f, err := fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_NOFOLLOW, 0600)
	if err != nil && fs.IsAccessDenied(err) {
		// If file is readonly, clear the readonly flag by resetting the
		// permissions of the file and try again
		// as the metadata will be set again in the second pass and the
		// readonly flag will be applied again if needed.
		if err = fs.ResetPermissions(path); err != nil {
			return nil, err
		}
		if f, err = fs.OpenFile(path, fs.O_WRONLY|fs.O_NOFOLLOW, 0600); err != nil {
			return nil, err
		}
	} else if err != nil && (errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EISDIR)) {
		// symlink or directory, try to remove it later on
		f = nil
	} else if err != nil {
		return nil, err
	}

	var fi os.FileInfo
	if f != nil {
		// stat to check that we've opened a regular file
		fi, err = f.Stat()
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	mustReplace := f == nil || !fi.Mode().IsRegular()
	if !mustReplace {
		ex := fs.ExtendedStat(fi)
		if ex.Links > 1 {
			// there is no efficient way to find out which other files might be linked to this file
			// thus nuke the existing file and start with a fresh one
			mustReplace = true
		}
	}

	if mustReplace {
		// close handle if we still have it
		if f != nil {
			if err := f.Close(); err != nil {
				return nil, err
			}
		}

		// not what we expected, try to get rid of it
		if allowRecursiveDelete {
			if err := fs.RemoveAll(path); err != nil {
				return nil, err
			}
		} else {
			if err := fs.Remove(path); err != nil {
				return nil, err
			}
		}
		// create a new file, pass O_EXCL to make sure there are no surprises
		f, err = fs.OpenFile(path, fs.O_CREATE|fs.O_WRONLY|fs.O_EXCL|fs.O_NOFOLLOW, 0600)
		if err != nil {
			return nil, err
		}
		fi, err = f.Stat()
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	return ensureSize(f, fi, createSize, sparse)
}

func ensureSize(f *os.File, fi os.FileInfo, createSize int64, sparse bool) (*os.File, error) {
	if sparse {
		err := truncateSparse(f, createSize)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	} else if fi.Size() > createSize {
		// file is too long must shorten it
		err := f.Truncate(createSize)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	} else if createSize > 0 {
		err := fs.PreallocateFile(f, createSize)
		if err != nil {
			// Just log the preallocate error but don't let it cause the restore process to fail.
			// Preallocate might return an error if the filesystem (implementation) does not
			// support preallocation or our parameters combination to the preallocate call
			// This should yield a syscall.ENOTSUP error, but some other errors might also
			// show up.
			debug.Log("Failed to preallocate %v with size %v: %v", f.Name(), createSize, err)
		}
	}
	return f, nil
}

func (w *filesWriter) writeToFile(path string, blob []byte, offset int64, createSize int64, sparse bool) error {
	bucket := &w.buckets[uint(xxhash.Sum64String(path))%uint(len(w.buckets))]

	acquireWriter := func() (*partialFile, error) {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if wr, ok := bucket.files[path]; ok {
			bucket.files[path].users++
			return wr, nil
		}

		// Check the global LRU cache for a cached file handle
		w.cacheMu.Lock()
		cached, ok := w.cache.Get(path)
		if ok {
			// mark as in use to prevent closing on remove call below
			cached.users++

			w.cache.Remove(path)
			w.cacheMu.Unlock()

			// Use the cached file handle
			bucket.files[path] = cached
			return cached, nil
		}
		w.cacheMu.Unlock()

		// Not in cache, open/create the file
		var f *os.File
		var err error
		if createSize >= 0 {
			f, err = createFile(path, createSize, sparse, w.allowRecursiveDelete)
			if err != nil {
				return nil, err
			}
		} else if f, err = openFile(path); err != nil {
			return nil, err
		}

		wr := &partialFile{File: f, users: 1, sparse: sparse}
		bucket.files[path] = wr

		return wr, nil
	}

	releaseWriter := func(wr *partialFile) error {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		bucket.files[path].users--
		if bucket.files[path].users == 0 {
			delete(bucket.files, path)
			// Add to cache to allow re-use. Cache will close files on overflow.
			w.cacheMu.Lock()
			w.cache.Add(path, wr)
			w.cacheMu.Unlock()
		}
		return nil
	}

	wr, err := acquireWriter()
	if err != nil {
		return err
	}

	_, err = wr.WriteAt(blob, offset)

	if err != nil {
		// ignore subsequent errors
		_ = releaseWriter(wr)
		return err
	}

	return releaseWriter(wr)
}

func (w *filesWriter) flush() {
	w.cacheMu.Lock()
	defer w.cacheMu.Unlock()

	w.cache.Purge()
}
