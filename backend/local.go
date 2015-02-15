package backend

import (
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/arrar"
)

const (
	dirMode         = 0700
	dataPath        = "data"
	snapshotPath    = "snapshots"
	treePath        = "trees"
	lockPath        = "locks"
	keyPath         = "keys"
	tempPath        = "tmp"
	versionFileName = "version"
)

var ErrWrongData = errors.New("wrong data returned by backend, checksum does not match")

type Local struct {
	p   string
	ver uint
}

// OpenLocal opens the local backend at dir.
func OpenLocal(dir string) (*Local, error) {
	items := []string{
		dir,
		filepath.Join(dir, dataPath),
		filepath.Join(dir, snapshotPath),
		filepath.Join(dir, treePath),
		filepath.Join(dir, lockPath),
		filepath.Join(dir, keyPath),
		filepath.Join(dir, tempPath),
	}

	// test if all necessary dirs and files are there
	for _, d := range items {
		if _, err := os.Stat(d); err != nil {
			return nil, fmt.Errorf("%s does not exist", d)
		}
	}

	// read version file
	f, err := os.Open(filepath.Join(dir, versionFileName))
	if err != nil {
		return nil, fmt.Errorf("unable to read version file: %v\n", err)
	}

	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	version, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return nil, fmt.Errorf("unable to convert version to integer: %v\n", err)
	}

	// check version
	if version != BackendVersion {
		return nil, fmt.Errorf("wrong version %d", version)
	}

	return &Local{p: dir, ver: uint(version)}, nil
}

// CreateLocal creates all the necessary files and directories for a new local
// backend at dir.
func CreateLocal(dir string) (*Local, error) {
	versionFile := filepath.Join(dir, versionFileName)
	dirs := []string{
		dir,
		filepath.Join(dir, dataPath),
		filepath.Join(dir, snapshotPath),
		filepath.Join(dir, treePath),
		filepath.Join(dir, lockPath),
		filepath.Join(dir, keyPath),
		filepath.Join(dir, tempPath),
	}

	// test if version file already exists
	_, err := os.Lstat(versionFile)
	if err == nil {
		return nil, errors.New("version file already exists")
	}

	// test if directories already exist
	for _, d := range dirs[1:] {
		if _, err := os.Stat(d); err == nil {
			return nil, fmt.Errorf("dir %s already exists", d)
		}
	}

	// create paths for data, refs and temp
	for _, d := range dirs {
		err := os.MkdirAll(d, dirMode)
		if err != nil {
			return nil, err
		}
	}

	// create version file
	f, err := os.Create(versionFile)
	if err != nil {
		return nil, err
	}

	_, err = f.Write([]byte(fmt.Sprintf("%d\n", BackendVersion)))
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	// open backend
	return OpenLocal(dir)
}

// Location returns this backend's location (the directory name).
func (b *Local) Location() string {
	return b.p
}

// Return temp directory in correct directory for this backend.
func (b *Local) tempFile() (*os.File, error) {
	return ioutil.TempFile(filepath.Join(b.p, tempPath), "temp-")
}

// Rename temp file to final name according to type and ID.
func (b *Local) renameFile(file *os.File, t Type, id ID) error {
	newname := b.filename(t, id)
	oldname := file.Name()

	if t == Data || t == Tree {
		// create directories if necessary, ignore errors
		os.MkdirAll(filepath.Dir(newname), dirMode)
	}

	return os.Rename(oldname, newname)
}

// Construct directory for given Type.
func (b *Local) dirname(t Type, id ID) string {
	var n string
	switch t {
	case Data:
		n = dataPath
		if id != nil {
			n = filepath.Join(dataPath, fmt.Sprintf("%02x", id[0]))
		}
	case Snapshot:
		n = snapshotPath
	case Tree:
		n = treePath
		if id != nil {
			n = filepath.Join(treePath, fmt.Sprintf("%02x", id[0]))
		}
	case Lock:
		n = lockPath
	case Key:
		n = keyPath
	}
	return filepath.Join(b.p, n)
}

// Create stores new content of type t and data and returns the ID. If the blob
// is already present, returns ErrAlreadyPresent and the blob's ID.
func (b *Local) Create(t Type, data []byte) (ID, error) {
	// TODO: make sure that tempfile is removed upon error

	// check if blob is already present in backend
	id := IDFromData(data)
	res, err := b.Test(t, id)
	if err != nil {
		return nil, arrar.Annotate(err, "test for presence")
	}

	if res {
		return id, ErrAlreadyPresent
	}

	// create tempfile in backend
	file, err := b.tempFile()
	if err != nil {
		return nil, err
	}

	// write data to tempfile
	_, err = file.Write(data)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	// return id
	err = b.renameFile(file, t, id)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// CreateFrom reads content from rd and stores it as type t. Returned is the
// storage ID. If the blob is already present, returns ErrAlreadyPresent and
// the blob's ID.
func (b *Local) CreateFrom(t Type, rd io.Reader) (ID, error) {
	// TODO: make sure that tempfile is removed upon error

	// check hash while writing
	hr := NewHashingReader(rd, newHash())

	// create tempfile in backend
	file, err := b.tempFile()
	if err != nil {
		return nil, err
	}

	// write data to tempfile
	_, err = io.Copy(file, hr)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	// get ID
	id := ID(hr.Sum(nil))

	// check for duplicate ID
	res, err := b.Test(t, id)
	if err != nil {
		return nil, arrar.Annotate(err, "test for presence")
	}

	if res {
		return id, ErrAlreadyPresent
	}

	// rename file
	err = b.renameFile(file, t, id)
	if err != nil {
		return nil, err
	}

	return id, nil
}

type localBlob struct {
	f       *os.File
	h       hash.Hash
	tw      io.Writer
	backend *Local
	tpe     Type
	id      ID
}

func (lb *localBlob) Close() error {
	err := lb.f.Close()
	if err != nil {
		return err
	}

	// get ID
	lb.id = ID(lb.h.Sum(nil))

	// check for duplicate ID
	res, err := lb.backend.Test(lb.tpe, lb.id)
	if err != nil {
		return fmt.Errorf("testing presence of ID %v failed: %v", lb.id, err)
	}

	if res {
		return ErrAlreadyPresent
	}

	// rename file
	err = lb.backend.renameFile(lb.f, lb.tpe, lb.id)
	if err != nil {
		return err
	}

	return nil
}

func (lb *localBlob) Write(p []byte) (int, error) {
	return lb.tw.Write(p)
}

func (lb *localBlob) ID() (ID, error) {
	if lb.id == nil {
		return nil, errors.New("blob is not closed, ID unavailable")
	}

	return lb.id, nil
}

// Create creates a new blob of type t. Blob implements io.WriteCloser. Once
// Close() has been called, ID() can be used to retrieve the ID. If the blob is
// already present, Close() returns ErrAlreadyPresent.
func (b *Local) CreateBlob(t Type) (Blob, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in backend
	file, err := b.tempFile()
	if err != nil {
		return nil, err
	}

	h := newHash()
	blob := localBlob{
		h:       h,
		tw:      io.MultiWriter(h, file),
		f:       file,
		backend: b,
		tpe:     t,
	}

	return &blob, nil
}

// Construct path for given Type and ID.
func (b *Local) filename(t Type, id ID) string {
	return filepath.Join(b.dirname(t, id), id.String())
}

// Get returns the content stored under the given ID. If the data doesn't match
// the requested ID, ErrWrongData is returned.
func (b *Local) Get(t Type, id ID) ([]byte, error) {
	if id == nil {
		return nil, errors.New("unable to load nil ID")
	}

	// try to open file
	file, err := os.Open(b.filename(t, id))
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// read all
	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// check id
	if !Hash(buf).Equal(id) {
		return nil, ErrWrongData
	}

	return buf, nil
}

// GetReader returns a reader that yields the content stored under the given
// ID. The content is not verified. The reader should be closed after draining
// it.
func (b *Local) GetReader(t Type, id ID) (io.ReadCloser, error) {
	if id == nil {
		return nil, errors.New("unable to load nil ID")
	}

	// try to open file
	file, err := os.Open(b.filename(t, id))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Test returns true if a blob of the given type and ID exists in the backend.
func (b *Local) Test(t Type, id ID) (bool, error) {
	// try to open file
	file, err := os.Open(b.filename(t, id))
	defer func() {
		file.Close()
	}()

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Remove removes the content stored at ID.
func (b *Local) Remove(t Type, id ID) error {
	return os.Remove(b.filename(t, id))
}

// List lists all objects of a given type.
func (b *Local) List(t Type) (IDs, error) {
	// TODO: use os.Open() and d.Readdirnames() instead of Glob()
	var pattern string
	if t == Data || t == Tree {
		pattern = filepath.Join(b.dirname(t, nil), "*", "*")
	} else {
		pattern = filepath.Join(b.dirname(t, nil), "*")
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	ids := make(IDs, 0, len(matches))

	for _, m := range matches {
		base := filepath.Base(m)

		if base == "" {
			continue
		}
		id, err := ParseID(base)

		if err != nil {
			continue
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// Version returns the version of this local backend.
func (b *Local) Version() uint {
	return b.ver
}

// Close closes the backend
func (b *Local) Close() error {
	return nil
}

// Delete removes the repository and all files.
func (b *Local) Delete() error {
	return os.RemoveAll(b.p)
}
