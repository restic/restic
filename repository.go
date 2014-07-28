package khepri

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
)

const (
	dirMode    = 0700
	objectPath = "objects"
	refPath    = "refs"
	tempPath   = "tmp"
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
		path.Join(r.path, objectPath),
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

// Put saves content and returns the ID.
func (r *DirRepository) Put(reader io.Reader) (ID, error) {
	// save contents to tempfile, hash while writing
	file, err := ioutil.TempFile(path.Join(r.path, tempPath), "temp-")
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
	filename := path.Join(r.path, objectPath, id.String())
	err = os.Rename(file.Name(), filename)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// PutFile saves a file's content to the repository and returns the ID.
func (r *DirRepository) PutFile(path string) (ID, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	return r.Put(f)
}

// PutRaw saves a []byte's content to the repository and returns the ID.
func (r *DirRepository) PutRaw(buf []byte) (ID, error) {
	// save contents to tempfile, hash while writing
	file, err := ioutil.TempFile(path.Join(r.path, tempPath), "temp-")
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
	filename := path.Join(r.path, objectPath, id.String())
	err = os.Rename(file.Name(), filename)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// Test returns true if the given ID exists in the repository.
func (r *DirRepository) Test(id ID) (bool, error) {
	// try to open file
	file, err := os.Open(path.Join(r.path, objectPath, id.String()))
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
func (r *DirRepository) Get(id ID) (io.Reader, error) {
	// try to open file
	file, err := os.Open(path.Join(r.path, objectPath, id.String()))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Remove removes the content stored at ID.
func (r *DirRepository) Remove(id ID) error {
	return os.Remove(path.Join(r.path, objectPath, id.String()))
}

// Unlink removes a named ID.
func (r *DirRepository) Unlink(name string) error {
	return os.Remove(path.Join(r.path, refPath, Name(name).Encode()))
}

// Link assigns a name to an ID. Name must be unique in this repository and ID must exist.
func (r *DirRepository) Link(name string, id ID) error {
	exist, err := r.Test(id)
	if err != nil {
		return err
	}

	if !exist {
		return ErrIDDoesNotExist
	}

	// create file, write id
	f, err := os.Create(path.Join(r.path, refPath, Name(name).Encode()))
	defer f.Close()

	if err != nil {
		return err
	}

	f.Write([]byte(hex.EncodeToString(id)))
	return nil
}

// Resolve returns the ID associated with the given name.
func (r *DirRepository) Resolve(name string) (ID, error) {
	f, err := os.Open(path.Join(r.path, refPath, Name(name).Encode()))
	defer f.Close()
	if err != nil {
		return nil, err
	}

	// read hex string
	l := r.hash().Size()
	buf := make([]byte, l*2)
	_, err = io.ReadFull(f, buf)

	if err != nil {
		return nil, err
	}

	id := make([]byte, l)
	_, err = hex.Decode(id, buf)
	if err != nil {
		return nil, err
	}

	return ID(id), nil
}
