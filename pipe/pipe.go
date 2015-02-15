package pipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func walk(path string, done chan struct{}, entCh chan<- Entry, dirCh chan<- Dir, res chan<- interface{}) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory, cannot walk: %s", path)
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
			// entCh <- Entry{Info: fi, Error: err, Result: ch}
			return err
		}

		if isDir(fi) {
			err = walk(subpath, done, entCh, dirCh, ch)
			if err != nil {
				return err
			}

		} else {
			entCh <- Entry{Info: fi, Path: subpath, Result: ch}
		}
	}

	dirCh <- Dir{Path: path, Info: info, Entries: entries, Result: res}
	return nil
}

// Walk takes a path and sends a Job for each file and directory it finds below
// the path. When the channel done is closed, processing stops.
func Walk(path string, done chan struct{}, entCh chan<- Entry, dirCh chan<- Dir) (<-chan interface{}, error) {
	resCh := make(chan interface{}, 1)
	err := walk(path, done, entCh, dirCh, resCh)
	close(entCh)
	close(dirCh)
	return resCh, err
}
