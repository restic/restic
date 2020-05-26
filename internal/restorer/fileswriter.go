package restorer

import (
	"os"
	"sync"

	"github.com/cespare/xxhash"
)

// writes blobs to target files.
// multiple files can be written to concurrently.
// multiple blobs can be concurrently written to the same file.
// TODO I am not 100% convinced this is necessary, i.e. it may be okay
//      to use multiple os.File to write to the same target file
type filesWriter struct {
	buckets []filesWriterBucket
}

type filesWriterBucket struct {
	lock  sync.Mutex
	files map[string]*partialFile
}

type partialFile struct {
	*os.File
	size  int64 // File size, tracked for sparse writes (not on Windows).
	users int   // Reference count.
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

func (w *filesWriter) writeToFile(path string, blob []byte, offset int64, create bool) error {
	bucket := &w.buckets[uint(xxhash.Sum64String(path))%uint(len(w.buckets))]

	acquireWriter := func() (*partialFile, error) {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if wr, ok := bucket.files[path]; ok {
			bucket.files[path].users++
			return wr, nil
		}

		var flags int
		if create {
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		} else {
			flags = os.O_WRONLY
		}

		f, err := os.OpenFile(path, flags, 0600)
		if err != nil {
			return nil, err
		}

		wr := &partialFile{File: f, users: 1}
		if !create {
			info, err := f.Stat()
			if err != nil {
				return nil, err
			}
			wr.size = info.Size()
		}
		bucket.files[path] = wr

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
		releaseWriter(wr)
		return err
	}

	return releaseWriter(wr)
}
