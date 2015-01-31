package restic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type Server struct {
	be  backend.Backend
	key *Key
}

func NewServer(be backend.Backend) Server {
	return Server{be: be}
}

func NewServerWithKey(be backend.Backend, key *Key) Server {
	return Server{be: be, key: key}
}

// Each lists all entries of type t in the backend and calls function f() with
// the id and data.
func (s Server) Each(t backend.Type, f func(id backend.ID, data []byte, err error)) error {
	return backend.Each(s.be, t, f)
}

// Each lists all entries of type t in the backend and calls function f() with
// the id.
func (s Server) EachID(t backend.Type, f func(backend.ID)) error {
	return backend.EachID(s.be, t, f)
}

// Find loads the list of all blobs of type t and searches for IDs which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func (s Server) Find(t backend.Type, prefix string) (backend.ID, error) {
	return backend.Find(s.be, t, prefix)
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func (s Server) FindSnapshot(id string) (backend.ID, error) {
	return backend.FindSnapshot(s.be, id)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (s Server) PrefixLength(t backend.Type) (int, error) {
	return backend.PrefixLength(s.be, t)
}

// Load tries to load and decrypt content identified by t and blob from the backend.
func (s Server) Load(t backend.Type, blob Blob) ([]byte, error) {
	// load data
	buf, err := s.Get(t, blob.Storage)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.StorageSize) {
		return nil, errors.New("Invalid storage length")
	}

	// decrypt
	buf, err = s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.Size) {
		return nil, errors.New("Invalid length")
	}

	// check SHA256 sum
	id := backend.Hash(buf)
	if !blob.ID.Equal(id) {
		return nil, fmt.Errorf("load %v: expected plaintext hash %v, got %v", blob.Storage, blob.ID, id)
	}

	return buf, nil
}

// Load tries to load and decrypt content identified by t and id from the backend.
func (s Server) LoadID(t backend.Type, storageID backend.ID) ([]byte, error) {
	// load data
	buf, err := s.Get(t, storageID)
	if err != nil {
		return nil, err
	}

	// decrypt
	buf, err = s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// LoadJSON calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (s Server) LoadJSON(t backend.Type, blob Blob, item interface{}) error {
	// load from backend
	buf, err := s.Load(t, blob)
	if err != nil {
		return err
	}

	// inflate and unmarshal
	err = json.Unmarshal(backend.Uncompress(buf), item)
	return err
}

// LoadJSONID calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (s Server) LoadJSONID(t backend.Type, storageID backend.ID, item interface{}) error {
	// load from backend
	buf, err := s.LoadID(t, storageID)
	if err != nil {
		return err
	}

	// inflate and unmarshal
	err = json.Unmarshal(backend.Uncompress(buf), item)
	return err
}

// Save encrypts data and stores it to the backend as type t.
func (s Server) Save(t backend.Type, data []byte, id backend.ID) (Blob, error) {
	if id == nil {
		// compute plaintext hash
		id = backend.Hash(data)
	}

	// create a new blob
	blob := Blob{
		ID:   id,
		Size: uint64(len(data)),
	}

	var ciphertext []byte

	// if the data is small enough, use a slice from the pool
	if len(data) <= maxCiphertextSize-ivSize-hmacSize {
		ciphertext = GetChunkBuf("ch.Save()")
		defer FreeChunkBuf("ch.Save()", ciphertext)
	} else {
		l := len(data) + ivSize + hmacSize

		debug.Log("Server.Save", "create large slice of %d bytes for ciphertext", l)

		// use a new slice
		ciphertext = make([]byte, l)
	}

	// encrypt blob
	n, err := s.Encrypt(ciphertext, data)
	if err != nil {
		return Blob{}, err
	}

	ciphertext = ciphertext[:n]

	// save blob
	sid, err := s.Create(t, ciphertext)
	if err != nil {
		return Blob{}, err
	}

	blob.Storage = sid
	blob.StorageSize = uint64(len(ciphertext))

	return blob, nil
}

// SaveJSON serialises item as JSON and uses Save() to store it to the backend as type t.
func (s Server) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	// convert to json
	data, err := json.Marshal(item)
	if err != nil {
		return Blob{}, err
	}

	// compress and save data
	return s.Save(t, backend.Compress(data), nil)
}

// Returns the backend used for this server.
func (s Server) Backend() backend.Backend {
	return s.be
}

func (s *Server) SearchKey(password string) error {
	key, err := SearchKey(*s, password)
	if err != nil {
		return err
	}

	s.key = key

	return nil
}

func (s Server) Decrypt(ciphertext []byte) ([]byte, error) {
	if s.key == nil {
		return nil, errors.New("key for server not set")
	}

	return s.key.Decrypt(ciphertext)
}

func (s Server) Encrypt(ciphertext, plaintext []byte) (int, error) {
	if s.key == nil {
		return 0, errors.New("key for server not set")
	}

	return s.key.Encrypt(ciphertext, plaintext)
}

func (s Server) Key() *Key {
	return s.key
}

// Each calls Each() with the given parameters, Decrypt() on the ciphertext
// and, on successful decryption, f with the plaintext.
func (s Server) EachDecrypted(t backend.Type, f func(backend.ID, []byte, error)) error {
	if s.key == nil {
		return errors.New("key for server not set")
	}

	return s.Each(t, func(id backend.ID, data []byte, e error) {
		if e != nil {
			f(id, nil, e)
			return
		}

		buf, err := s.key.Decrypt(data)
		if err != nil {
			f(id, nil, err)
			return
		}

		f(id, buf, nil)
	})
}

// Proxy methods to backend

func (s Server) List(t backend.Type) (backend.IDs, error) {
	return s.be.List(t)
}

func (s Server) Get(t backend.Type, id backend.ID) ([]byte, error) {
	return s.be.Get(t, id)
}

func (s Server) Create(t backend.Type, data []byte) (backend.ID, error) {
	return s.be.Create(t, data)
}

func (s Server) Test(t backend.Type, id backend.ID) (bool, error) {
	return s.be.Test(t, id)
}

func (s Server) Remove(t backend.Type, id backend.ID) error {
	return s.be.Remove(t, id)
}

func (s Server) Close() error {
	return s.be.Close()
}

func (s Server) Delete() error {
	if b, ok := s.be.(backend.Deleter); ok {
		return b.Delete()
	}

	return errors.New("Delete() called for backend that does not implement this method")
}
