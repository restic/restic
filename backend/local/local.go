package local

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/restic/restic/backend"
)

var ErrWrongData = errors.New("wrong data returned by backend, checksum does not match")

type Local struct {
	p   string
	ver uint
	id  string
}

// Open opens the local backend at dir.
func Open(dir string) (*Local, error) {
	items := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Trees),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}

	// test if all necessary dirs and files are there
	for _, d := range items {
		if _, err := os.Stat(d); err != nil {
			return nil, fmt.Errorf("%s does not exist", d)
		}
	}

	// read version file
	f, err := os.Open(filepath.Join(dir, backend.Paths.Version))
	if err != nil {
		return nil, fmt.Errorf("unable to read version file: %v\n", err)
	}

	var version uint
	n, err := fmt.Fscanf(f, "%d", &version)
	if err != nil {
		return nil, err
	}

	if n != 1 {
		return nil, errors.New("could not read version from file")
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	// check version
	if version != backend.Version {
		return nil, fmt.Errorf("wrong version %d", version)
	}

	// read ID
	f, err = os.Open(filepath.Join(dir, backend.Paths.ID))
	if err != nil {
		return nil, err
	}

	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(string(buf))
	if err != nil {
		return nil, err
	}

	return &Local{p: dir, ver: version, id: id}, nil
}

// Create creates all the necessary files and directories for a new local
// backend at dir.
func Create(dir string) (*Local, error) {
	versionFile := filepath.Join(dir, backend.Paths.Version)
	idFile := filepath.Join(dir, backend.Paths.ID)
	dirs := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Trees),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}

	// test if files already exist
	_, err := os.Lstat(versionFile)
	if err == nil {
		return nil, errors.New("version file already exists")
	}

	_, err = os.Lstat(idFile)
	if err == nil {
		return nil, errors.New("id file already exists")
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

	// create version file
	f, err := os.Create(versionFile)
	if err != nil {
		return nil, err
	}

	_, err = fmt.Fprintf(f, "%d\n", backend.Version)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	// create ID file
	id := make([]byte, sha256.Size)
	_, err = rand.Read(id)
	if err != nil {
		return nil, err
	}

	f, err = os.Create(idFile)
	if err != nil {
		return nil, err
	}

	_, err = fmt.Fprintln(f, hex.EncodeToString(id))
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
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
	if t == backend.Data || t == backend.Tree {
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
	case backend.Tree:
		n = backend.Paths.Trees
		if len(name) > 2 {
			n = filepath.Join(n, name[:2])
		}
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
// goroutine ist started for this. If the channel done is closed, sending
// stops.
func (b *Local) List(t backend.Type, done <-chan struct{}) <-chan string {
	// TODO: use os.Open() and d.Readdirnames() instead of Glob()
	var pattern string
	if t == backend.Data || t == backend.Tree {
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

// Version returns the version of this local backend.
func (b *Local) Version() uint {
	return b.ver
}

// ID returns the ID of this local backend.
func (b *Local) ID() string {
	return b.id
}

// Delete removes the repository and all files.
func (b *Local) Delete() error { return os.RemoveAll(b.p) }

// Close does nothing
func (b *Local) Close() error { return nil }
