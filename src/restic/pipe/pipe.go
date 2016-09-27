package pipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"restic/errors"

	"restic/debug"
	"restic/fs"
)

type Result interface{}

type Job interface {
	Path() string
	Fullpath() string
	Error() error
	Info() os.FileInfo

	Result() chan<- Result
}

type Entry struct {
	basedir string
	path    string
	info    os.FileInfo
	error   error
	result  chan<- Result

	// points to the old node if available, interface{} is used to prevent
	// circular import
	Node interface{}
}

func (e Entry) Path() string          { return e.path }
func (e Entry) Fullpath() string      { return filepath.Join(e.basedir, e.path) }
func (e Entry) Error() error          { return e.error }
func (e Entry) Info() os.FileInfo     { return e.info }
func (e Entry) Result() chan<- Result { return e.result }

type Dir struct {
	basedir string
	path    string
	error   error
	info    os.FileInfo

	Entries [](<-chan Result)
	result  chan<- Result
}

func (e Dir) Path() string          { return e.path }
func (e Dir) Fullpath() string      { return filepath.Join(e.basedir, e.path) }
func (e Dir) Error() error          { return e.error }
func (e Dir) Info() os.FileInfo     { return e.info }
func (e Dir) Result() chan<- Result { return e.result }

// readDirNames reads the directory named by dirname and returns
// a sorted list of directory entries.
// taken from filepath/path.go
func readDirNames(dirname string) ([]string, error) {
	f, err := fs.Open(dirname)
	if err != nil {
		return nil, errors.Wrap(err, "Open")
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, errors.Wrap(err, "Readdirnames")
	}
	sort.Strings(names)
	return names, nil
}

// SelectFunc returns true for all items that should be included (files and
// dirs). If false is returned, files are ignored and dirs are not even walked.
type SelectFunc func(item string, fi os.FileInfo) bool

func walk(basedir, dir string, selectFunc SelectFunc, done <-chan struct{}, jobs chan<- Job, res chan<- Result) (excluded bool) {
	debug.Log("start on %q, basedir %q", dir, basedir)

	relpath, err := filepath.Rel(basedir, dir)
	if err != nil {
		panic(err)
	}

	info, err := fs.Lstat(dir)
	if err != nil {
		err = errors.Wrap(err, "Lstat")
		debug.Log("error for %v: %v, res %p", dir, err, res)
		select {
		case jobs <- Dir{basedir: basedir, path: relpath, info: info, error: err, result: res}:
		case <-done:
		}
		return
	}

	if !selectFunc(dir, info) {
		debug.Log("file %v excluded by filter, res %p", dir, res)
		excluded = true
		return
	}

	if !info.IsDir() {
		debug.Log("sending file job for %v, res %p", dir, res)
		select {
		case jobs <- Entry{info: info, basedir: basedir, path: relpath, result: res}:
		case <-done:
		}
		return
	}

	debug.RunHook("pipe.readdirnames", dir)
	names, err := readDirNames(dir)
	if err != nil {
		debug.Log("Readdirnames(%v) returned error: %v, res %p", dir, err, res)
		select {
		case <-done:
		case jobs <- Dir{basedir: basedir, path: relpath, info: info, error: err, result: res}:
		}
		return
	}

	// Insert breakpoint to allow testing behaviour with vanishing files
	// between Readdir() and lstat()
	debug.RunHook("pipe.walk1", relpath)

	entries := make([]<-chan Result, 0, len(names))

	for _, name := range names {
		subpath := filepath.Join(dir, name)

		fi, statErr := fs.Lstat(subpath)
		if !selectFunc(subpath, fi) {
			debug.Log("file %v excluded by filter", subpath)
			continue
		}

		ch := make(chan Result, 1)
		entries = append(entries, ch)

		if statErr != nil {
			statErr = errors.Wrap(statErr, "Lstat")
			debug.Log("sending file job for %v, err %v, res %p", subpath, err, res)
			select {
			case jobs <- Entry{info: fi, error: statErr, basedir: basedir, path: filepath.Join(relpath, name), result: ch}:
			case <-done:
				return
			}
			continue
		}

		// Insert breakpoint to allow testing behaviour with vanishing files
		// between walk and open
		debug.RunHook("pipe.walk2", filepath.Join(relpath, name))

		walk(basedir, subpath, selectFunc, done, jobs, ch)
	}

	debug.Log("sending dirjob for %q, basedir %q, res %p", dir, basedir, res)
	select {
	case jobs <- Dir{basedir: basedir, path: relpath, info: info, Entries: entries, result: res}:
	case <-done:
	}

	return
}

// cleanupPath is used to clean a path. For a normal path, a slice with just
// the path is returned. For special cases such as "." and "/" the list of
// names within those paths is returned.
func cleanupPath(path string) ([]string, error) {
	path = filepath.Clean(path)
	if filepath.Dir(path) != path {
		return []string{path}, nil
	}

	paths, err := readDirNames(path)
	if err != nil {
		return nil, err
	}

	for i, p := range paths {
		paths[i] = filepath.Join(path, p)
	}

	return paths, nil
}

// Walk sends a Job for each file and directory it finds below the paths. When
// the channel done is closed, processing stops.
func Walk(walkPaths []string, selectFunc SelectFunc, done chan struct{}, jobs chan<- Job, res chan<- Result) {
	var paths []string

	for _, p := range walkPaths {
		ps, err := cleanupPath(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Readdirnames(%v): %v, skipping\n", p, err)
			debug.Log("Readdirnames(%v) returned error: %v, skipping", p, err)
			continue
		}

		paths = append(paths, ps...)
	}

	debug.Log("start on %v", paths)
	defer func() {
		debug.Log("output channel closed")
		close(jobs)
	}()

	entries := make([]<-chan Result, 0, len(paths))
	for _, path := range paths {
		debug.Log("start walker for %v", path)
		ch := make(chan Result, 1)
		excluded := walk(filepath.Dir(path), path, selectFunc, done, jobs, ch)

		if excluded {
			debug.Log("walker for %v done, it was excluded by the filter", path)
			continue
		}

		entries = append(entries, ch)
		debug.Log("walker for %v done", path)
	}

	debug.Log("sending root node, res %p", res)
	select {
	case <-done:
		return
	case jobs <- Dir{Entries: entries, result: res}:
	}

	debug.Log("walker done")
}

// Split feeds all elements read from inChan to dirChan and entChan.
func Split(inChan <-chan Job, dirChan chan<- Dir, entChan chan<- Entry) {
	debug.Log("start")
	defer debug.Log("done")

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
