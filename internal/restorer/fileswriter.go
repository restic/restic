package restorer

import (
	"os"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// Writes blobs to output files. Each file is written sequentially,
// start to finish, but multiple files can be written to concurrently.
// Implementation allows virtually unlimited number of logically open
// files, but number of phisically open files will never exceed number
// of concurrent writeToFile invocations plus cacheCap.
type filesWriter struct {
	lock       sync.Mutex          // guards concurrent access to open files cache
	inprogress map[string]struct{} // (logically) opened file writers
	cache      map[string]*os.File // cache of open files
	cacheCap   int                 // max number of cached open files
}

func newFilesWriter(cacheCap int) *filesWriter {
	return &filesWriter{
		inprogress: make(map[string]struct{}),
		cache:      make(map[string]*os.File),
		cacheCap:   cacheCap,
	}
}

func (w *filesWriter) writeToFile(path string, blob []byte) error {
	// First writeToFile invocation for any given path will:
	// - create and open the file
	// - write the blob to the file
	// - cache the open file if there is space, close the file otherwise
	// Subsequent invocations will:
	// - remove the open file from the cache _or_ open the file for append
	// - write the blob to the file
	// - cache the open file if there is space, close the file otherwise
	// The idea is to cap maximum number of open files with minimal
	// coordination among concurrent writeToFile invocations (note that
	// writeToFile never touches somebody else's open file).

	// TODO measure if caching is useful (likely depends on operating system
	// and hardware configuration)
	acquireWriter := func() (*os.File, error) {
		w.lock.Lock()
		defer w.lock.Unlock()
		if wr, ok := w.cache[path]; ok {
			debug.Log("Used cached writer for %s", path)
			delete(w.cache, path)
			return wr, nil
		}
		var flags int
		if _, append := w.inprogress[path]; append {
			flags = os.O_APPEND | os.O_WRONLY
		} else {
			w.inprogress[path] = struct{}{}
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		}
		wr, err := os.OpenFile(path, flags, 0600)
		if err != nil {
			return nil, err
		}
		debug.Log("Opened writer for %s", path)
		return wr, nil
	}
	cacheOrCloseWriter := func(wr *os.File) {
		w.lock.Lock()
		defer w.lock.Unlock()
		if len(w.cache) < w.cacheCap {
			w.cache[path] = wr
		} else {
			wr.Close()
		}
	}

	wr, err := acquireWriter()
	if err != nil {
		return err
	}
	n, err := wr.Write(blob)
	cacheOrCloseWriter(wr)
	if err != nil {
		return err
	}
	if n != len(blob) {
		return errors.Errorf("error writing file %v: wrong length written, want %d, got %d", path, len(blob), n)
	}
	return nil
}

func (w *filesWriter) close(path string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if wr, ok := w.cache[path]; ok {
		wr.Close()
		delete(w.cache, path)
	}
	delete(w.inprogress, path)
}
