package backend

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/arrar"
)

const (
	dirMode         = 0700
	blobPath        = "blobs"
	snapshotPath    = "snapshots"
	treePath        = "trees"
	lockPath        = "locks"
	keyPath         = "keys"
	tempPath        = "tmp"
	versionFileName = "version"
)

type Local struct {
	p   string
	ver uint
}

// OpenLocal opens the local backend at dir.
func OpenLocal(dir string) (*Local, error) {
	items := []string{
		dir,
		filepath.Join(dir, blobPath),
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

	if version != BackendVersion {
		return nil, fmt.Errorf("wrong version %d", version)
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
		filepath.Join(dir, blobPath),
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

	// create paths for blobs, refs and temp
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

	_, err = f.Write([]byte(strconv.Itoa(BackendVersion)))
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
	filename := filepath.Join(b.dir(t), id.String())
	return os.Rename(file.Name(), filename)
}

// Construct directory for given Type.
func (b *Local) dir(t Type) string {
	var n string
	switch t {
	case Blob:
		n = blobPath
	case Snapshot:
		n = snapshotPath
	case Tree:
		n = treePath
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

// Construct path for given Type and ID.
func (b *Local) filename(t Type, id ID) string {
	return filepath.Join(b.dir(t), id.String())
}

// Get returns the content stored under the given ID.
func (b *Local) Get(t Type, id ID) ([]byte, error) {
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

	return buf, nil
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
	pattern := filepath.Join(b.dir(t), "*")

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
