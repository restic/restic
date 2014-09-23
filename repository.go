package khepri

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	dirMode        = 0700
	blobPath       = "blobs"
	refPath        = "refs"
	tempPath       = "tmp"
	configFileName = "config.json"
)

var (
	ErrIDDoesNotExist = errors.New("ID does not exist")
)

// Name stands for the alias given to an ID.
type Name string

func (n Name) Encode() string {
	return url.QueryEscape(string(n))
}

type HashFunc func() hash.Hash

type Repository struct {
	path   string
	hash   HashFunc
	config *Config
}

type Config struct {
	Salt string
	N    uint
	R    uint `json:"r"`
	P    uint `json:"p"`
}

// TODO: figure out scrypt values on the fly depending on the current
// hardware.
const (
	scrypt_N        = 65536
	scrypt_r        = 8
	scrypt_p        = 1
	scrypt_saltsize = 64
)

type Type int

const (
	TYPE_BLOB = iota
	TYPE_REF
)

func NewTypeFromString(s string) Type {
	switch s {
	case "blob":
		return TYPE_BLOB
	case "ref":
		return TYPE_REF
	}

	panic(fmt.Sprintf("unknown type %q", s))
}

func (t Type) String() string {
	switch t {
	case TYPE_BLOB:
		return "blob"
	case TYPE_REF:
		return "ref"
	}

	panic(fmt.Sprintf("unknown type %d", t))
}

// NewRepository opens a dir-baked repository at the given path.
func NewRepository(path string) (*Repository, error) {
	var err error

	d := &Repository{
		path: path,
		hash: sha256.New,
	}

	d.config, err = d.read_config()
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (r *Repository) read_config() (*Config, error) {
	// try to open config file
	f, err := os.Open(path.Join(r.path, configFileName))
	if err != nil {
		return nil, err
	}

	cfg := new(Config)
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// CreateRepository creates all the necessary files and directories for the
// Repository.
func CreateRepository(p string) (*Repository, error) {
	dirs := []string{
		p,
		path.Join(p, blobPath),
		path.Join(p, refPath),
		path.Join(p, tempPath),
	}

	var configfile = path.Join(p, configFileName)

	// test if repository directories or config file already exist
	if _, err := os.Stat(configfile); err == nil {
		return nil, fmt.Errorf("config file %s already exists", configfile)
	}

	for _, d := range dirs[1:] {
		if _, err := os.Stat(d); err == nil {
			return nil, fmt.Errorf("dir %s already exists", d)
		}
	}

	// create initial json configuration
	cfg := &Config{
		N: scrypt_N,
		R: scrypt_r,
		P: scrypt_p,
	}

	// generate salt
	buf := make([]byte, scrypt_saltsize)
	n, err := rand.Read(buf)
	if n != scrypt_saltsize || err != nil {
		panic("unable to read enough random bytes for salt")
	}
	cfg.Salt = hex.EncodeToString(buf)

	// create ps for blobs, refs and temp
	for _, dir := range dirs {
		err := os.MkdirAll(dir, dirMode)
		if err != nil {
			return nil, err
		}
	}

	// write config file
	f, err := os.Create(configfile)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	s, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	_, err = f.Write(s)
	if err != nil {
		return nil, err
	}

	// open repository
	return NewRepository(p)
}

// SetHash changes the hash function used for deriving IDs. Default is SHA256.
func (r *Repository) SetHash(h HashFunc) {
	r.hash = h
}

// Path returns the directory used for this repository.
func (r *Repository) Path() string {
	return r.path
}

// Return temp directory in correct directory for this repository.
func (r *Repository) tempFile() (*os.File, error) {
	return ioutil.TempFile(path.Join(r.path, tempPath), "temp-")
}

// Rename temp file to final name according to type and ID.
func (r *Repository) renameFile(file *os.File, t Type, id ID) error {
	filename := path.Join(r.dir(t), id.String())
	return os.Rename(file.Name(), filename)
}

// Construct directory for given Type.
func (r *Repository) dir(t Type) string {
	switch t {
	case TYPE_BLOB:
		return path.Join(r.path, blobPath)
	case TYPE_REF:
		return path.Join(r.path, refPath)
	}

	panic(fmt.Sprintf("unknown type %d", t))
}

// Construct path for given Type and ID.
func (r *Repository) filename(t Type, id ID) string {
	return path.Join(r.dir(t), id.String())
}

// Test returns true if the given ID exists in the repository.
func (r *Repository) Test(t Type, id ID) (bool, error) {
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
func (r *Repository) Get(t Type, id ID) (io.ReadCloser, error) {
	// try to open file
	file, err := os.Open(r.filename(t, id))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Remove removes the content stored at ID.
func (r *Repository) Remove(t Type, id ID) error {
	return os.Remove(r.filename(t, id))
}

type IDs []ID

// Lists all objects of a given type.
func (r *Repository) List(t Type) (IDs, error) {
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
