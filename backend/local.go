package backend

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	dirMode      = 0700
	blobPath     = "blobs"
	snapshotPath = "snapshots"
	treePath     = "trees"
	lockPath     = "locks"
	keyPath      = "keys"
	tempPath     = "tmp"
)

type Local struct {
	p string
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

	return &Local{p: dir}, nil
}

// CreateLocal creates all the necessary files and directories for a new local
// backend at dir.
func CreateLocal(dir string) (*Local, error) {
	dirs := []string{
		dir,
		filepath.Join(dir, blobPath),
		filepath.Join(dir, snapshotPath),
		filepath.Join(dir, treePath),
		filepath.Join(dir, lockPath),
		filepath.Join(dir, keyPath),
		filepath.Join(dir, tempPath),
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

	// open repository
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

// Create stores new content of type t and data and returns the ID.
func (b *Local) Create(t Type, data []byte) (ID, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in repository
	var err error
	file, err := b.tempFile()
	if err != nil {
		return nil, err
	}

	// write data to tempfile
	_, err = file.Write(data)
	if err != nil {
		return nil, err
	}

	// close tempfile, return id
	id := IDFromData(data)
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
