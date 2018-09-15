package restorer

import (
	"io"
	"os"
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

type filesWriter struct {
	lock       sync.Mutex          // guards concurrent access
	inprogress map[string]struct{} // (logically) opened file writers
	writers    simplelru.LRUCache  // key: string, value: *os.File
}

func newFilesWriter(count int) *filesWriter {
	writers, _ := simplelru.NewLRU(count, func(key interface{}, value interface{}) {
		value.(*os.File).Close()
		debug.Log("Closed and purged cached writer for %v", key)
	})
	return &filesWriter{inprogress: make(map[string]struct{}), writers: writers}
}

func (w *filesWriter) writeToFile(path string, buf []byte) error {
	acquireWriter := func() (io.Writer, error) {
		w.lock.Lock()
		defer w.lock.Unlock()
		if wr, ok := w.writers.Get(path); ok {
			debug.Log("Used cached writer for %s", path)
			return wr.(*os.File), nil
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
		w.writers.Add(path, wr)
		debug.Log("Opened and cached writer for %s", path)
		return wr, nil
	}

	wr, err := acquireWriter()
	if err != nil {
		return err
	}
	n, err := wr.Write(buf)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return errors.Errorf("error writing file %v: wrong length written, want %d, got %d", path, len(buf), n)
	}
	return nil
}

func (w *filesWriter) close(path string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.writers.Remove(path)
	delete(w.inprogress, path)
}
