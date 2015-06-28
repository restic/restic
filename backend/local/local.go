package local

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/restic/restic/backend"
)

var ErrWrongData = errors.New("wrong data returned by backend, checksum does not match")

type Local struct {
	p string
}

// Open opens the local backend at dir.
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

	return &Local{p: dir}, nil
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
	_, err := os.Lstat(backend.Paths.Config)
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// test if directories already exist
	for _, d := range dirs[1:] {
		if _, err := os.Stat(d); err == nil {
			return nil, fmt.Errorf("dir %s already exists", d)
		}
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

// Return temp directory in correct directory for this backend.
func (b *Local) tempFile() (*os.File, error) {
	return ioutil.TempFile(filepath.Join(b.p, backend.Paths.Temp), "temp-")
}

type localBlob struct {
	f       *os.File
	size    uint
	final   bool
	basedir string
}

func (lb *localBlob) Write(p []byte) (int, error) {
	if lb.final {
		return 0, errors.New("blob already closed")
	}

	n, err := lb.f.Write(p)
	lb.size += uint(n)
	return n, err
}

func (lb *localBlob) Size() uint {
	return lb.size
}

func (lb *localBlob) Finalize(t backend.Type, name string) error {
	if lb.final {
		return errors.New("Already finalized")
	}

	lb.final = true

	err := lb.f.Close()
	if err != nil {
		return fmt.Errorf("local: file.Close: %v", err)
	}

	f := filename(lb.basedir, t, name)

	// create directories if necessary, ignore errors
	if t == backend.Data {
		os.MkdirAll(filepath.Dir(f), backend.Modes.Dir)
	}

	// test if new path already exists
	if _, err := os.Stat(f); err == nil {
		return fmt.Errorf("Close(): file %v already exists", f)
	}

	if err := os.Rename(lb.f.Name(), f); err != nil {
		return err
	}

	// set mode to read-only
	fi, err := os.Stat(f)
	if err != nil {
		return err
	}

	return os.Chmod(f, fi.Mode()&os.FileMode(^uint32(0222)))
}

// Create creates a new Blob. The data is available only after Finalize()
// has been called on the returned Blob.
func (b *Local) Create() (backend.Blob, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in backend
	file, err := b.tempFile()
	if err != nil {
		return nil, err
	}

	blob := localBlob{
		f:       file,
		basedir: b.p,
	}

	return &blob, nil
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

// Get returns a reader that yields the content stored under the given
// name. The reader should be closed after draining it.
func (b *Local) Get(t backend.Type, name string) (io.ReadCloser, error) {
	return os.Open(filename(b.p, t, name))
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length. If length is 0, the reader reads until EOF.
func (b *Local) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	f, err := os.Open(filename(b.p, t, name))
	if err != nil {
		return nil, err
	}

	_, err = f.Seek(int64(offset), 0)
	if err != nil {
		return nil, err
	}

	if length == 0 {
		return f, nil
	}

	return backend.LimitReadCloser(f, int64(length)), nil
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
	return os.Remove(filename(b.p, t, name))
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
func (b *Local) Delete() error { return os.RemoveAll(b.p) }

// Close does nothing
func (b *Local) Close() error { return nil }
