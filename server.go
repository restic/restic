package restic

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
)

type Server struct {
	be  backend.Backend
	key *Key
}

func NewServer(be backend.Backend) Server {
	return Server{be: be}
}

func (s *Server) SetKey(k *Key) {
	s.key = k
}

// ChunkerPolynomial returns the secret polynomial used for content defined chunking.
func (s *Server) ChunkerPolynomial() chunker.Pol {
	return chunker.Pol(s.key.Master().ChunkerPolynomial)
}

// Find loads the list of all blobs of type t and searches for names which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func (s Server) Find(t backend.Type, prefix string) (string, error) {
	return backend.Find(s.be, t, prefix)
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func (s Server) FindSnapshot(name string) (string, error) {
	return backend.FindSnapshot(s.be, name)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (s Server) PrefixLength(t backend.Type) (int, error) {
	return backend.PrefixLength(s.be, t)
}

// Load tries to load and decrypt content identified by t and blob from the
// backend. If the blob specifies an ID, the decrypted plaintext is checked
// against this ID. The same goes for blob.Size and blob.StorageSize: If they
// are set to a value > 0, this value is checked.
func (s Server) Load(t backend.Type, blob Blob) ([]byte, error) {
	// load data
	rd, err := s.be.Get(t, blob.Storage.String())
	if err != nil {
		return nil, err
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	// check hash
	if !backend.Hash(buf).Equal(blob.Storage) {
		return nil, errors.New("invalid data returned")
	}

	// check length
	if blob.StorageSize > 0 && len(buf) != int(blob.StorageSize) {
		return nil, errors.New("Invalid storage length")
	}

	// decrypt
	buf, err = s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check length
	if blob.Size > 0 && len(buf) != int(blob.Size) {
		return nil, errors.New("Invalid length")
	}

	// check SHA256 sum
	if blob.ID != nil {
		id := backend.Hash(buf)
		if !blob.ID.Equal(id) {
			return nil, fmt.Errorf("load %v: expected plaintext hash %v, got %v", blob.Storage, blob.ID, id)
		}
	}

	return buf, nil
}

// Load tries to load and decrypt content identified by t and id from the backend.
func (s Server) LoadID(t backend.Type, storageID backend.ID) ([]byte, error) {
	return s.Load(t, Blob{Storage: storageID})
}

// LoadJSON calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (s Server) LoadJSON(t backend.Type, blob Blob, item interface{}) error {
	buf, err := s.Load(t, blob)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LoadJSONID calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (s Server) LoadJSONID(t backend.Type, id backend.ID, item interface{}) error {
	// read
	rd, err := s.be.Get(t, id.String())
	if err != nil {
		return err
	}
	defer rd.Close()

	// decrypt
	decryptRd, err := s.key.DecryptFrom(rd)
	defer decryptRd.Close()
	if err != nil {
		return err
	}

	// decode
	decoder := json.NewDecoder(decryptRd)
	err = decoder.Decode(item)
	if err != nil {
		return err
	}

	return nil
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
	if len(data) <= maxCiphertextSize-crypto.Extension {
		ciphertext = GetChunkBuf("ch.Save()")
		defer FreeChunkBuf("ch.Save()", ciphertext)
	} else {
		l := len(data) + crypto.Extension

		debug.Log("Server.Save", "create large slice of %d bytes for ciphertext", l)

		// use a new slice
		ciphertext = make([]byte, l)
	}

	// encrypt blob
	ciphertext, err := s.Encrypt(ciphertext, data)
	if err != nil {
		return Blob{}, err
	}

	// compute ciphertext hash
	sid := backend.Hash(ciphertext)

	// save blob
	backendBlob, err := s.be.Create()
	if err != nil {
		return Blob{}, err
	}

	_, err = backendBlob.Write(ciphertext)
	if err != nil {
		return Blob{}, err
	}

	err = backendBlob.Finalize(t, sid.String())
	if err != nil {
		return Blob{}, err
	}

	blob.Storage = sid
	blob.StorageSize = uint64(len(ciphertext))

	return blob, nil
}

// SaveFrom encrypts data read from rd and stores it to the backend as type t.
func (s Server) SaveFrom(t backend.Type, id backend.ID, length uint, rd io.Reader) (Blob, error) {
	if id == nil {
		return Blob{}, errors.New("id is nil")
	}

	backendBlob, err := s.be.Create()
	if err != nil {
		return Blob{}, err
	}

	hw := backend.NewHashingWriter(backendBlob, sha256.New())
	encWr := s.key.EncryptTo(hw)

	_, err = io.Copy(encWr, rd)
	if err != nil {
		return Blob{}, err
	}

	// finish encryption
	err = encWr.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("EncryptedWriter.Close(): %v", err)
	}

	// finish backend blob
	sid := backend.ID(hw.Sum(nil))
	err = backendBlob.Finalize(t, sid.String())
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.Close(): %v", err)
	}

	return Blob{
		ID:          id,
		Size:        uint64(length),
		Storage:     sid,
		StorageSize: uint64(backendBlob.Size()),
	}, nil
}

// SaveJSON serialises item as JSON and encrypts and saves it in the backend as
// type t.
func (s Server) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	backendBlob, err := s.be.Create()
	if err != nil {
		return Blob{}, fmt.Errorf("Create: %v", err)
	}

	storagehw := backend.NewHashingWriter(backendBlob, sha256.New())
	encWr := s.key.EncryptTo(storagehw)
	plainhw := backend.NewHashingWriter(encWr, sha256.New())

	enc := json.NewEncoder(plainhw)
	err = enc.Encode(item)
	if err != nil {
		return Blob{}, fmt.Errorf("json.NewEncoder: %v", err)
	}

	// finish encryption
	err = encWr.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("EncryptedWriter.Close(): %v", err)
	}

	// finish backend blob
	sid := backend.ID(storagehw.Sum(nil))
	err = backendBlob.Finalize(t, sid.String())
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.Close(): %v", err)
	}

	id := backend.ID(plainhw.Sum(nil))

	return Blob{
		ID:          id,
		Size:        uint64(plainhw.Size()),
		Storage:     sid,
		StorageSize: uint64(backendBlob.Size()),
	}, nil
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

	return s.key.Decrypt([]byte{}, ciphertext)
}

func (s Server) Encrypt(ciphertext, plaintext []byte) ([]byte, error) {
	if s.key == nil {
		return nil, errors.New("key for server not set")
	}

	return s.key.Encrypt(ciphertext, plaintext)
}

func (s Server) Key() *Key {
	return s.key
}

type ServerStats struct {
	Blobs, Trees uint
}

// Stats returns statistics for this backend and the server.
func (s Server) Stats() (ServerStats, error) {
	blobs := backend.NewIDSet()

	// load all trees, in parallel
	worker := func(wg *sync.WaitGroup, b <-chan Blob) {
		for blob := range b {
			tree, err := LoadTree(s, blob)
			// ignore error and advance to next tree
			if err != nil {
				return
			}

			for _, id := range tree.Map.StorageIDs() {
				blobs.Insert(id)
			}
		}
		wg.Done()
	}

	blobCh := make(chan Blob)

	// start workers
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go worker(&wg, blobCh)
	}

	// list ids
	trees := 0
	done := make(chan struct{})
	defer close(done)
	for name := range s.List(backend.Tree, done) {
		trees++
		id, err := backend.ParseID(name)
		if err != nil {
			debug.Log("Server.Stats", "unable to parse name %v as id: %v", name, err)
			continue
		}
		blobCh <- Blob{Storage: id}
	}

	close(blobCh)

	// wait for workers
	wg.Wait()

	return ServerStats{Blobs: uint(blobs.Len()), Trees: uint(trees)}, nil
}

// Count returns the number of blobs of a given type in the backend.
func (s Server) Count(t backend.Type) (n int) {
	for _ = range s.List(t, nil) {
		n++
	}

	return
}

// Proxy methods to backend

func (s Server) List(t backend.Type, done <-chan struct{}) <-chan string {
	return s.be.List(t, done)
}

func (s Server) Test(t backend.Type, name string) (bool, error) {
	return s.be.Test(t, name)
}

func (s Server) Remove(t backend.Type, name string) error {
	return s.be.Remove(t, name)
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

func (s Server) ID() string {
	return s.be.ID()
}

func (s Server) Location() string {
	return s.be.Location()
}
