package pipe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/restic/restic/debug"
)

type Entry struct {
	Path   string
	Info   os.FileInfo
	Error  error
	Result chan<- interface{}
}

type Dir struct {
	Path  string
	Error error
	Info  os.FileInfo

	Entries [](<-chan interface{})
	Result  chan<- interface{}
}

// readDirNames reads the directory named by dirname and returns
// a sorted list of directory entries.
// taken from filepath/path.go
func readDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func isDir(fi os.FileInfo) bool {
	return fi.IsDir()
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

var errCancelled = errors.New("walk cancelled")

func walk(path string, done chan struct{}, jobs chan<- interface{}, res chan<- interface{}) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		select {
		case jobs <- Entry{Info: info, Path: path, Result: res}:
		case <-done:
			return errCancelled
		}
		return nil
	}

	names, err := readDirNames(path)
	if err != nil {
		return err
	}

	entries := make([]<-chan interface{}, 0, len(names))

	for _, name := range names {
		subpath := filepath.Join(path, name)

		ch := make(chan interface{}, 1)
		entries = append(entries, ch)

		fi, err := os.Lstat(subpath)
		if err != nil {
			select {
			case jobs <- Entry{Info: fi, Error: err, Result: ch}:
			case <-done:
				return errCancelled
			}
			continue
		}

		if isDir(fi) {
			err = walk(subpath, done, jobs, ch)
			if err != nil {
				return err
			}

		} else {
			select {
			case jobs <- Entry{Info: fi, Path: subpath, Result: ch}:
			case <-done:
				return errCancelled
			}
		}
	}

	select {
	case jobs <- Dir{Path: path, Info: info, Entries: entries, Result: res}:
	case <-done:
		return errCancelled
	}
	return nil
}

// Walk sends a Job for each file and directory it finds below the paths. When
// the channel done is closed, processing stops.
func Walk(paths []string, done chan struct{}, jobs chan<- interface{}) (<-chan interface{}, error) {
	resCh := make(chan interface{}, 1)
	defer func() {
		close(resCh)
		close(jobs)
		debug.Log("pipe.Walk", "output channel closed")
	}()

	entries := make([]<-chan interface{}, 0, len(paths))
	for _, path := range paths {
		debug.Log("pipe.Walk", "start walker for %v", path)
		ch := make(chan interface{}, 1)
		entries = append(entries, ch)
		err := walk(path, done, jobs, ch)
		if err != nil {
			return nil, err
		}
		debug.Log("pipe.Walk", "walker for %v done", path)
	}
	resCh <- Dir{Entries: entries}
	return resCh, nil
}

// Split feeds all elements read from inChan to dirChan and entChan.
func Split(inChan <-chan interface{}, dirChan chan<- Dir, entChan chan<- Entry) {
	debug.Log("pipe.Split", "start")
	defer debug.Log("pipe.Split", "done")

	inCh := inChan
	dirCh := dirChan
	entCh := entChan

	var (
		dir Dir
		ent Entry
	)

	// deactivate sending until we received at least one job
	dirCh = nil
	entCh = nil
	for {
		select {
		case job, ok := <-inCh:
			if !ok {
				// channel is closed
				return
			}

			if job == nil {
				panic("nil job received")
			}

			// disable receiving until the current job has been sent
			inCh = nil

			switch j := job.(type) {
			case Dir:
				dir = j
				dirCh = dirChan
			case Entry:
				ent = j
				entCh = entChan
			default:
				panic(fmt.Sprintf("unknown job type %v", j))
			}
		case dirCh <- dir:
			// disable sending, re-enable receiving
			dirCh = nil
			inCh = inChan
		case entCh <- ent:
			// disable sending, re-enable receiving
			entCh = nil
			inCh = inChan
		}
	}
}
