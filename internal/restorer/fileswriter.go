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
	lock       sync.Mutex             // guards concurrent access
	inprogress map[*fileInfo]struct{} // (logically) opened file writers
	writers    simplelru.LRUCache     // key: *fileInfo, value: *os.File
}

func newFilesWriter(count int) *filesWriter {
	writers, _ := simplelru.NewLRU(count, func(key interface{}, value interface{}) {
		value.(*os.File).Close()
		debug.Log("Closed and purged cached writer for %v", key)
	})
	return &filesWriter{inprogress: make(map[*fileInfo]struct{}), writers: writers}
}

func (w *filesWriter) writeToFile(file *fileInfo, buf []byte) error {
	acquireWriter := func() (io.Writer, error) {
		w.lock.Lock()
		defer w.lock.Unlock()
		if wr, ok := w.writers.Get(file); ok {
			debug.Log("Used cached writer for %s", file.path)
			return wr.(*os.File), nil
		}
		var flags int
		if _, append := w.inprogress[file]; append {
			flags = os.O_APPEND | os.O_WRONLY
		} else {
			w.inprogress[file] = struct{}{}
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		}
		wr, err := os.OpenFile(file.path, flags, 0600)
		if err != nil {
			return nil, err
		}
		w.writers.Add(file, wr)
		debug.Log("Opened and cached writer for %s", file.path)
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
		return errors.Errorf("error writing file %v: wrong length written, want %d, got %d", file.path, len(buf), n)
	}
	return nil
}

func (w *filesWriter) close(file *fileInfo) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.writers.Remove(file)
	delete(w.inprogress, file)
}
