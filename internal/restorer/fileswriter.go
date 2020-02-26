package restorer

import (
	"bytes"
	"os"
	"runtime"
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

func (w *filesWriter) writeToFile(path string, blob []byte, offset int64, create bool) error {
	bucket := &w.buckets[uint(xxhash.Sum64String(path))%uint(len(w.buckets))]

	acquireWriter := func() (*os.File, error) {
		bucket.lock.Lock()
		defer bucket.lock.Unlock()

		if wr, ok := bucket.files[path]; ok {
			bucket.users[path]++
			return wr, nil
		}

		var flags int
		if create {
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

	if writeByTruncate() && allZero(blob) {
		err = w.writeZeros(wr, blob, offset)
	} else {
		_, err = wr.WriteAt(blob, offset)
	}

	if err != nil {
		releaseWriter(wr)
		return err
	}

	return releaseWriter(wr)
}

// writeZeros writes blob, which must be all zeros, to offset in wr.
func (w *filesWriter) writeZeros(wr *os.File, blob []byte, offset int64) error {
	fi, err := wr.Stat()
	if err != nil {
		return err
	}

	if fi.Size() >= offset+int64(len(blob)) {
		// A previous writeToFile call will already have allocated the block.
		// Since the file was newly created, it will be all zeros.
		return nil
	}

	err = wr.Truncate(offset + int64(len(blob)))
	if err != nil {
		// Retry. If this fails too, at least
		// we get the error message from WriteAt.
		_, err = wr.WriteAt(blob, offset)
	}
	return err
}

// writeByTruncate returns true if we can write zeros to a file by truncating
// it to more than its current size. That may create a sparse file, depending
// on the underlying file system.
func writeByTruncate() bool { return runtime.GOOS != "windows" }

func allZero(p []byte) bool {
	var zeros [2048]byte

	for len(p) > 0 {
		n := len(zeros)
		if n > len(p) {
			n = len(p)
		}
		if !bytes.Equal(p[:n], zeros[:n]) {
			return false
		}
		p = p[n:]
	}
	return true
}
