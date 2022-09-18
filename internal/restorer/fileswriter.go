package restorer

import (
	"os"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/restic/restic/internal/debug"
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
	files map[string]*os.File
	users map[string]int
}

func newFilesWriter(count int) *filesWriter {
	buckets := make([]filesWriterBucket, count)
	for b := 0; b < count; b++ {
		buckets[b].files = make(map[string]*os.File)
		buckets[b].users = make(map[string]int)
	}
	return &filesWriter{
		buckets: buckets,
	}
}

func (w *filesWriter) writeToFile(path string, blob []byte, offset int64, createSize int64) error {
	bucket := &w.buckets[uint(xxhash.Sum64String(path))%uint(len(w.buckets))]

	acquireWriter := func() (*os.File, error) {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if wr, ok := bucket.files[path]; ok {
			bucket.users[path]++
			return wr, nil
		}

		var flags int
		if createSize >= 0 {
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		} else {
			flags = os.O_WRONLY
		}

		wr, err := os.OpenFile(path, flags, 0600)
		if err != nil {
			return nil, err
		}

		bucket.files[path] = wr
		bucket.users[path] = 1

		if createSize >= 0 {
			err := preallocateFile(wr, createSize)
			if err != nil {
				// Just log the preallocate error but don't let it cause the restore process to fail.
				// Preallocate might return an error if the filesystem (implementation) does not
				// support preallocation or our parameters combination to the preallocate call
				// This should yield a syscall.ENOTSUP error, but some other errors might also
				// show up.
				debug.Log("Failed to preallocate %v with size %v: %v", path, createSize, err)
			}
		}

		return wr, nil
	}

	releaseWriter := func(wr *os.File) error {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if bucket.users[path] == 1 {
			delete(bucket.files, path)
			delete(bucket.users, path)
			return wr.Close()
		}
		bucket.users[path]--
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
