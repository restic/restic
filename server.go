package restic

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

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
	return s.LoadJSONID(t, blob.Storage, item)
}

var (
	zEmptyString = []byte("x\x9C\x03\x00\x00\x00\x00\x01")
)

var zReaderPool = sync.Pool{
	New: func() interface{} {
		rd, err := zlib.NewReader(bytes.NewReader(zEmptyString))
		if err != nil {
			// shouldn't happen
			panic(err)
		}
		return rd
	},
}

type zReader interface {
	io.ReadCloser
	zlib.Resetter
}

// LoadJSONID calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (s Server) LoadJSONID(t backend.Type, storageID backend.ID, item interface{}) error {
	// read
	rd, err := s.GetReader(t, storageID)
	defer rd.Close()
	if err != nil {
		return err
	}

	// decrypt
	decryptRd, err := s.key.DecryptFrom(rd)
	defer decryptRd.Close()
	if err != nil {
		return err
	}

	// unzip
	br := decryptRd.(flate.Reader)

	unzipRd := zReaderPool.Get().(zReader)
	err = unzipRd.Reset(br, nil)
	defer func() {
		unzipRd.Close()
		zReaderPool.Put(unzipRd)
	}()
	if err != nil {
		return err
	}

	// decode
	decoder := json.NewDecoder(unzipRd)
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
	backendBlob, err := s.Create(t)
	if err != nil {
		return Blob{}, err
	}

	_, err = backendBlob.Write(ciphertext)
	if err != nil {
		return Blob{}, err
	}

	err = backendBlob.Close()
	if err != nil {
		return Blob{}, err
	}

	sid, err := backendBlob.ID()
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

	backendBlob, err := s.Create(t)
	if err != nil {
		return Blob{}, err
	}

	encWr := s.key.EncryptTo(backendBlob)

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
	err = backendBlob.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.Close(): %v", err)
	}

	storageID, err := backendBlob.ID()
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.ID(): %v", err)
	}

	return Blob{
		ID:          id,
		Size:        uint64(length),
		Storage:     storageID,
		StorageSize: uint64(backendBlob.Size()),
	}, nil
}

var zWriterPool = sync.Pool{
	New: func() interface{} {
		return zlib.NewWriter(nil)
	},
}

// SaveJSON serialises item as JSON and encrypts and saves it in the backend as
// type t.
func (s Server) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	backendBlob, err := s.Create(t)
	if err != nil {
		return Blob{}, fmt.Errorf("Create: %v", err)
	}

	encWr := s.key.EncryptTo(backendBlob)
	wr := zWriterPool.Get().(*zlib.Writer)
	defer zWriterPool.Put(wr)
	wr.Reset(encWr)
	if err != nil {
		return Blob{}, fmt.Errorf("zlib.NewWriter: %v", err)
	}

	hw := backend.NewHashingWriter(wr, sha256.New())

	enc := json.NewEncoder(hw)
	err = enc.Encode(item)
	if err != nil {
		return Blob{}, fmt.Errorf("json.NewEncoder: %v", err)
	}

	// flush zlib writer
	err = wr.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("zlib.Writer.Close(): %v", err)
	}

	// finish encryption
	err = encWr.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("EncryptedWriter.Close(): %v", err)
	}

	// finish backend blob
	err = backendBlob.Close()
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.Close(): %v", err)
	}

	id := hw.Sum(nil)
	storageID, err := backendBlob.ID()
	if err != nil {
		return Blob{}, fmt.Errorf("backend.Blob.ID(): %v", err)
	}

	return Blob{
		ID:          id,
		Size:        uint64(hw.Size()),
		Storage:     storageID,
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

func (s Server) Encrypt(ciphertext, plaintext []byte) (int, error) {
	if s.key == nil {
		return 0, errors.New("key for server not set")
	}

	return s.key.Encrypt(ciphertext, plaintext)
}

func (s Server) Key() *Key {
	return s.key
}

type ServerStats struct {
	Blobs, Trees uint
	Bytes        uint64
}

// Stats returns statistics for this backend and the server.
func (s Server) Stats() (ServerStats, error) {
	blobs := backend.NewIDSet()

	// load all trees, in parallel
	worker := func(wg *sync.WaitGroup, c <-chan backend.ID) {
		for id := range c {
			tree, err := LoadTree(s, id)
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

	idCh := make(chan backend.ID)

	// start workers
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go worker(&wg, idCh)
	}

	// list ids
	trees := 0
	err := s.EachID(backend.Tree, func(id backend.ID) {
		trees++
		idCh <- id
	})

	close(idCh)

	// wait for workers
	wg.Wait()

	return ServerStats{Blobs: uint(blobs.Len()), Trees: uint(trees)}, err
}

// Count counts the number of objects of type t in the backend.
func (s Server) Count(t backend.Type) (int, error) {
	l, err := s.be.List(t)
	if err != nil {
		return 0, err
	}

	return len(l), nil
}

// Proxy methods to backend

func (s Server) List(t backend.Type) (backend.IDs, error) {
	return s.be.List(t)
}

func (s Server) Get(t backend.Type, id backend.ID) ([]byte, error) {
	return s.be.Get(t, id)
}

func (s Server) GetReader(t backend.Type, id backend.ID) (io.ReadCloser, error) {
	return s.be.GetReader(t, id)
}

func (s Server) Create(t backend.Type) (backend.Blob, error) {
	return s.be.Create(t)
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
