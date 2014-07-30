package khepri

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

const (
	dirMode  = 0700
	blobPath = "blobs"
	refPath  = "refs"
	tempPath = "tmp"
)

var (
	ErrIDDoesNotExist = errors.New("ID does not exist")
)

// Name stands for the alias given to an ID.
type Name string

func (n Name) Encode() string {
	return url.QueryEscape(string(n))
}

type DirRepository struct {
	path string
	hash func() hash.Hash
}

type Type int

const (
	TypeUnknown = iota
	TypeBlob
	TypeRef
)

func NewTypeFromString(s string) Type {
	switch s {
	case "blob":
		return TypeBlob
	case "ref":
		return TypeRef
	}

	panic(fmt.Sprintf("unknown type %q", s))
}

func (t Type) String() string {
	switch t {
	case TypeBlob:
		return "blob"
	case TypeRef:
		return "ref"
	}

	panic(fmt.Sprintf("unknown type %d", t))
}

// NewDirRepository creates a new dir-baked repository at the given path.
func NewDirRepository(path string) (*DirRepository, error) {
	d := &DirRepository{
		path: path,
		hash: sha256.New,
	}

	err := d.create()

	if err != nil {
		return nil, err
	}

	return d, nil
}

func (r *DirRepository) create() error {
	dirs := []string{
		r.path,
		path.Join(r.path, blobPath),
		path.Join(r.path, refPath),
		path.Join(r.path, tempPath),
	}

	for _, dir := range dirs {
		err := os.MkdirAll(dir, dirMode)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetHash changes the hash function used for deriving IDs. Default is SHA256.
func (r *DirRepository) SetHash(h func() hash.Hash) {
	r.hash = h
}

// Path returns the directory used for this repository.
func (r *DirRepository) Path() string {
	return r.path
}

// Return temp directory in correct directory for this repository.
func (r *DirRepository) tempFile() (*os.File, error) {
	return ioutil.TempFile(path.Join(r.path, tempPath), "temp-")
}

// Rename temp file to final name according to type and ID.
func (r *DirRepository) renameFile(file *os.File, t Type, id ID) error {
	filename := path.Join(r.dir(t), id.String())
	return os.Rename(file.Name(), filename)
}

// Put saves content and returns the ID.
func (r *DirRepository) Put(t Type, reader io.Reader) (ID, error) {
	// save contents to tempfile, hash while writing
	file, err := r.tempFile()
	if err != nil {
		return nil, err
	}

	rd := NewHashingReader(reader, r.hash)
	_, err = io.Copy(file, rd)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	// move file to final name using hash of contents
	id := ID(rd.Hash())
	err = r.renameFile(file, t, id)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// Construct directory for given Type.
func (r *DirRepository) dir(t Type) string {
	switch t {
	case TypeBlob:
		return path.Join(r.path, blobPath)
	case TypeRef:
		return path.Join(r.path, refPath)
	}

	panic(fmt.Sprintf("unknown type %d", t))
}

// Construct path for given Type and ID.
func (r *DirRepository) filename(t Type, id ID) string {
	return path.Join(r.dir(t), id.String())
}

// PutFile saves a file's content to the repository and returns the ID.
func (r *DirRepository) PutFile(path string) (ID, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	return r.Put(TypeBlob, f)
}

// PutRaw saves a []byte's content to the repository and returns the ID.
func (r *DirRepository) PutRaw(t Type, buf []byte) (ID, error) {
	// save contents to tempfile, hash while writing
	file, err := r.tempFile()
	if err != nil {
		return nil, err
	}

	wr := NewHashingWriter(file, r.hash)
	_, err = wr.Write(buf)
	if err != nil {
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}

	// move file to final name using hash of contents
	id := ID(wr.Hash())
	err = r.renameFile(file, t, id)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// Test returns true if the given ID exists in the repository.
func (r *DirRepository) Test(t Type, id ID) (bool, error) {
	// try to open file
	file, err := os.Open(r.filename(t, id))
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

// Get returns a reader for the content stored under the given ID.
func (r *DirRepository) Get(t Type, id ID) (io.Reader, error) {
	// try to open file
	file, err := os.Open(r.filename(t, id))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Remove removes the content stored at ID.
func (r *DirRepository) Remove(t Type, id ID) error {
	return os.Remove(r.filename(t, id))
}

type IDs []ID

// Lists all objects of a given type.
func (r *DirRepository) ListIDs(t Type) (IDs, error) {
	// TODO: use os.Open() and d.Readdirnames() instead of Glob()
	pattern := path.Join(r.dir(t), "*")

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

func (ids IDs) Len() int {
	return len(ids)
}

func (ids IDs) Less(i, j int) bool {
	if len(ids[i]) < len(ids[j]) {
		return true
	}

	for k, b := range ids[i] {
		if b == ids[j][k] {
			continue
		}

		if b < ids[j][k] {
			return true
		} else {
			return false
		}
	}

	return false
}

func (ids IDs) Swap(i, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}
