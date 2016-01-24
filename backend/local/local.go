package local

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

// Local is a backend in a local directory.
type Local struct {
	p    string
	mu   sync.Mutex
	open map[string][]*os.File // Contains open files. Guarded by 'mu'.
}

// Open opens the local backend as specified by config.
func Open(dir string) (*Local, error) {
	items := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Index),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}

	// test if all necessary dirs are there
	for _, d := range items {
		if _, err := os.Stat(d); err != nil {
			return nil, fmt.Errorf("%s does not exist", d)
		}
	}

	return &Local{p: dir, open: make(map[string][]*os.File)}, nil
}

// Create creates all the necessary files and directories for a new local
// backend at dir. Afterwards a new config blob should be created.
func Create(dir string) (*Local, error) {
	dirs := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Index),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}

	// test if config file already exists
	_, err := os.Lstat(filepath.Join(dir, backend.Paths.Config))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data, refs and temp
	for _, d := range dirs {
		err := os.MkdirAll(d, backend.Modes.Dir)
		if err != nil {
			return nil, err
		}
	}

	// open backend
	return Open(dir)
}

// Location returns this backend's location (the directory name).
func (b *Local) Location() string {
	return b.p
}

// Construct path for given Type and name.
func filename(base string, t backend.Type, name string) string {
	if t == backend.Config {
		return filepath.Join(base, "config")
	}

	return filepath.Join(dirname(base, t, name), name)
}

// Construct directory for given Type.
func dirname(base string, t backend.Type, name string) string {
	var n string
	switch t {
	case backend.Data:
		n = backend.Paths.Data
		if len(name) > 2 {
			n = filepath.Join(n, name[:2])
		}
	case backend.Snapshot:
		n = backend.Paths.Snapshots
	case backend.Index:
		n = backend.Paths.Index
	case backend.Lock:
		n = backend.Paths.Locks
	case backend.Key:
		n = backend.Paths.Keys
	}
	return filepath.Join(base, n)
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (b *Local) Load(h backend.Handle, p []byte, off int64) (n int, err error) {
	if err := h.Valid(); err != nil {
		return 0, err
	}

	f, err := os.Open(filename(b.p, h.Type, h.Name))
	if err != nil {
		return 0, err
	}

	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}()

	if off > 0 {
		_, err = f.Seek(off, 0)
		if err != nil {
			return 0, err
		}
	}

	return io.ReadFull(f, p)
}

// Save stores data in the backend at the handle.
func (b *Local) Save(h backend.Handle, p []byte) (err error) {
	if err := h.Valid(); err != nil {
		return err
	}

	tmpfile, err := ioutil.TempFile(filepath.Join(b.p, backend.Paths.Temp), "temp-")
	if err != nil {
		return err
	}

	debug.Log("local.Save", "save %v (%d bytes) to %v", h, len(p), tmpfile.Name())

	n, err := tmpfile.Write(p)
	if err != nil {
		return err
	}

	if n != len(p) {
		return errors.New("not all bytes writen")
	}

	if err = tmpfile.Sync(); err != nil {
		return err
	}

	err = tmpfile.Close()
	if err != nil {
		return err
	}

	f := filename(b.p, h.Type, h.Name)

	// test if new path already exists
	if _, err := os.Stat(f); err == nil {
		return fmt.Errorf("Rename(): file %v already exists", f)
	}

	// create directories if necessary, ignore errors
	if h.Type == backend.Data {
		err = os.MkdirAll(filepath.Dir(f), backend.Modes.Dir)
		if err != nil {
			return err
		}
	}

	err = os.Rename(tmpfile.Name(), f)
	debug.Log("local.Save", "save %v: rename %v -> %v: %v",
		h, filepath.Base(tmpfile.Name()), filepath.Base(f), err)

	if err != nil {
		return err
	}

	// set mode to read-only
	fi, err := os.Stat(f)
	if err != nil {
		return err
	}

	return setNewFileMode(f, fi)
}

// Stat returns information about a blob.
func (b *Local) Stat(h backend.Handle) (backend.BlobInfo, error) {
	if err := h.Valid(); err != nil {
		return backend.BlobInfo{}, err
	}

	fi, err := os.Stat(filename(b.p, h.Type, h.Name))
	if err != nil {
		return backend.BlobInfo{}, err
	}

	return backend.BlobInfo{Size: fi.Size()}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (b *Local) Test(t backend.Type, name string) (bool, error) {
	_, err := os.Stat(filename(b.p, t, name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *Local) Remove(t backend.Type, name string) error {
	// close all open files we may have.
	fn := filename(b.p, t, name)
	b.mu.Lock()
	open, _ := b.open[fn]
	for _, file := range open {
		file.Close()
	}
	b.open[fn] = nil
	b.mu.Unlock()

	// reset read-only flag
	err := os.Chmod(fn, 0666)
	if err != nil {
		return err
	}

	return os.Remove(fn)
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (b *Local) List(t backend.Type, done <-chan struct{}) <-chan string {
	// TODO: use os.Open() and d.Readdirnames() instead of Glob()
	var pattern string
	if t == backend.Data {
		pattern = filepath.Join(dirname(b.p, t, ""), "*", "*")
	} else {
		pattern = filepath.Join(dirname(b.p, t, ""), "*")
	}

	ch := make(chan string)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		close(ch)
		return ch
	}

	for i := range matches {
		matches[i] = filepath.Base(matches[i])
	}

	sort.Strings(matches)

	go func() {
		defer close(ch)
		for _, m := range matches {
			if m == "" {
				continue
			}

			select {
			case ch <- m:
			case <-done:
				return
			}
		}
	}()

	return ch
}

// Delete removes the repository and all files.
func (b *Local) Delete() error {
	b.Close()
	return os.RemoveAll(b.p)
}

// Close closes all open files.
// They may have been closed already,
// so we ignore all errors.
func (b *Local) Close() error {
	b.mu.Lock()
	for _, open := range b.open {
		for _, file := range open {
			file.Close()
		}
	}
	b.open = make(map[string][]*os.File)
	b.mu.Unlock()
	return nil
}
