package restorer

import (
	"os"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// writes blobs to target files.
// multiple files can be written to concurrently.
// multiple blobs can be concurrently written to the same file.
// TODO I am not 100% convinced this is necessary, i.e. it may be okay
// to use multiple os.File to write to the same target file
type filesWriter struct {
	buckets []filesWriterBucket
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

func newFilesWriter(count int) *filesWriter {
	buckets := make([]filesWriterBucket, count)
	for b := 0; b < count; b++ {
		buckets[b].files = make(map[string]*partialFile)
	}
	return &filesWriter{
		buckets: buckets,
	}
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
		var f *os.File
		var err error
		if createSize >= 0 {
			if f, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); err != nil {
				if fs.IsAccessDenied(err) {
					// If file is readonly, clear the readonly flag by resetting the
					// permissions of the file and try again
					// as the metadata will be set again in the second pass and the
					// readonly flag will be applied again if needed.
					if err = fs.ResetPermissions(path); err != nil {
						return nil, err
					}
					if f, err = os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0600); err != nil {
						return nil, err
					}
				} else {
					return nil, err
				}
			}
		} else if f, err = os.OpenFile(path, os.O_WRONLY, 0600); err != nil {
			return nil, err
		}

		wr := &partialFile{File: f, users: 1, sparse: sparse}
		bucket.files[path] = wr

		if createSize >= 0 {
			if sparse {
				err = truncateSparse(f, createSize)
				if err != nil {
					return nil, err
				}
			} else {
				err := fs.PreallocateFile(wr.File, createSize)
				if err != nil {
					// Just log the preallocate error but don't let it cause the restore process to fail.
					// Preallocate might return an error if the filesystem (implementation) does not
					// support preallocation or our parameters combination to the preallocate call
					// This should yield a syscall.ENOTSUP error, but some other errors might also
					// show up.
					debug.Log("Failed to preallocate %v with size %v: %v", path, createSize, err)
				}
			}
		}

		return wr, nil
	}

	releaseWriter := func(wr *partialFile) error {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if bucket.files[path].users == 1 {
			delete(bucket.files, path)
			return wr.Close()
		}
		bucket.files[path].users--
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
