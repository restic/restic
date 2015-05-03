package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/restic/restic/pack"
)

// Config contains the configuration for a repository.
type Config struct {
	Version           uint        `json:"version"`
	ID                string      `json:"id"`
	ChunkerPolynomial chunker.Pol `json:"chunker_polynomial"`
}

// Server is used to access a repository in a backend.
type Server struct {
	be      backend.Backend
	Config  Config
	key     *crypto.Key
	keyName string
	idx     *Index

	pm    sync.Mutex
	packs []*pack.Packer
}

func NewServer(be backend.Backend) *Server {
	return &Server{
		be:  be,
		idx: NewIndex(),
	}
}

// Find loads the list of all blobs of type t and searches for names which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func (s *Server) Find(t backend.Type, prefix string) (string, error) {
	return backend.Find(s.be, t, prefix)
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func (s *Server) FindSnapshot(name string) (string, error) {
	return backend.FindSnapshot(s.be, name)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (s *Server) PrefixLength(t backend.Type) (int, error) {
	return backend.PrefixLength(s.be, t)
}

// Load tries to load and decrypt content identified by t and id from the
// backend.
func (s *Server) Load(t backend.Type, id backend.ID) ([]byte, error) {
	debug.Log("Server.Load", "load %v with id %v", t, id.Str())

	// load blob from pack
	rd, err := s.be.Get(t, id.String())
	if err != nil {
		debug.Log("Server.Load", "error loading %v: %v", id.Str(), err)
		return nil, err
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	err = rd.Close()
	if err != nil {
		return nil, err
	}

	// check hash
	if !backend.Hash(buf).Equal(id) {
		return nil, errors.New("invalid data returned")
	}

	// decrypt
	plain, err := s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	return plain, nil
}

// LoadBlob tries to load and decrypt content identified by t and id from a
// pack from the backend.
func (s *Server) LoadBlob(t pack.BlobType, id backend.ID) ([]byte, error) {
	debug.Log("Server.LoadBlob", "load %v with id %v", t, id.Str())
	// lookup pack
	packID, tpe, offset, length, err := s.idx.Lookup(id)
	if err != nil {
		debug.Log("Server.LoadBlob", "id %v not found in index: %v", id.Str(), err)
		return nil, err
	}

	if tpe != t {
		debug.Log("Server.LoadBlob", "wrong type returned for %v: wanted %v, got %v", id.Str(), t, tpe)
		return nil, fmt.Errorf("blob has wrong type %v (wanted: %v)", tpe, t)
	}

	debug.Log("Server.LoadBlob", "id %v found in pack %v at offset %v (length %d)", id.Str(), packID.Str(), offset, length)

	// load blob from pack
	rd, err := s.be.GetReader(backend.Data, packID.String(), offset, length)
	if err != nil {
		debug.Log("Server.LoadBlob", "error loading pack %v for %v: %v", packID.Str(), id.Str(), err)
		return nil, err
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	err = rd.Close()
	if err != nil {
		return nil, err
	}

	// decrypt
	plain, err := s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check hash
	if !backend.Hash(plain).Equal(id) {
		return nil, errors.New("invalid data returned")
	}

	return plain, nil
}

// LoadJSONEncrypted decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (s *Server) LoadJSONUnpacked(t backend.Type, id backend.ID, item interface{}) error {
	// load blob from backend
	rd, err := s.be.Get(t, id.String())
	if err != nil {
		return err
	}
	defer rd.Close()

	// decrypt
	decryptRd, err := crypto.DecryptFrom(s.key, rd)
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

// LoadJSONPack calls LoadBlob() to load a blob from the backend, decrypt the
// data and afterwards call json.Unmarshal on the item.
func (s *Server) LoadJSONPack(t pack.BlobType, id backend.ID, item interface{}) error {
	// lookup pack
	packID, _, offset, length, err := s.idx.Lookup(id)
	if err != nil {
		return err
	}

	// load blob from pack
	rd, err := s.be.GetReader(backend.Data, packID.String(), offset, length)
	if err != nil {
		return err
	}
	defer rd.Close()

	// decrypt
	decryptRd, err := crypto.DecryptFrom(s.key, rd)
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

const minPackSize = 4 * chunker.MiB
const maxPackSize = 16 * chunker.MiB
const maxPackers = 200

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (s *Server) findPacker(size uint) (*pack.Packer, error) {
	s.pm.Lock()
	defer s.pm.Unlock()

	// search for a suitable packer
	if len(s.packs) > 0 {
		debug.Log("Server.findPacker", "searching packer for %d bytes\n", size)
		for i, p := range s.packs {
			if p.Size()+size < maxPackSize {
				debug.Log("Server.findPacker", "found packer %v", p)
				// remove from list
				s.packs = append(s.packs[:i], s.packs[i+1:]...)
				return p, nil
			}
		}
	}

	// no suitable packer found, return new
	blob, err := s.be.Create()
	if err != nil {
		return nil, err
	}
	debug.Log("Server.findPacker", "create new pack %p", blob)
	return pack.NewPacker(s.key, blob), nil
}

// insertPacker appends p to s.packs.
func (s *Server) insertPacker(p *pack.Packer) {
	s.pm.Lock()
	defer s.pm.Unlock()

	s.packs = append(s.packs, p)
	debug.Log("Server.insertPacker", "%d packers\n", len(s.packs))
}

// savePacker stores p in the backend.
func (s *Server) savePacker(p *pack.Packer) error {
	debug.Log("Server.savePacker", "save packer with %d blobs\n", p.Count())
	_, err := p.Finalize()
	if err != nil {
		return err
	}

	// move file to the final location
	sid := p.ID()
	err = p.Writer().(backend.Blob).Finalize(backend.Data, sid.String())
	if err != nil {
		debug.Log("Server.savePacker", "blob Finalize() error: %v", err)
		return err
	}

	debug.Log("Server.savePacker", "saved as %v", sid.Str())

	// update blobs in the index
	for _, b := range p.Blobs() {
		debug.Log("Server.savePacker", "  updating blob %v to pack %v", b.ID.Str(), sid.Str())
		s.idx.Store(b.Type, b.ID, sid, b.Offset, uint(b.Length))
	}

	return nil
}

// countPacker returns the number of open (unfinished) packers.
func (s *Server) countPacker() int {
	s.pm.Lock()
	defer s.pm.Unlock()

	return len(s.packs)
}

// Save encrypts data and stores it to the backend as type t. If data is small
// enough, it will be packed together with other small blobs.
func (s *Server) Save(t pack.BlobType, data []byte, id backend.ID) (backend.ID, error) {
	if id == nil {
		// compute plaintext hash
		id = backend.Hash(data)
	}

	debug.Log("Server.Save", "save id %v (%v, %d bytes)", id.Str(), t, len(data))

	// get buf from the pool
	ciphertext := getBuf()
	defer freeBuf(ciphertext)

	// encrypt blob
	ciphertext, err := s.Encrypt(ciphertext, data)
	if err != nil {
		return nil, err
	}

	// find suitable packer and add blob
	packer, err := s.findPacker(uint(len(ciphertext)))
	if err != nil {
		return nil, err
	}

	// save ciphertext
	packer.Add(t, id, bytes.NewReader(ciphertext))

	// add this id to the index, although we don't know yet in which pack it
	// will be saved, the entry will be updated when the pack is written.
	s.idx.Store(t, id, nil, 0, 0)
	debug.Log("Server.Save", "saving stub for %v (%v) in index", id.Str, t)

	// if the pack is not full enough and there are less than maxPackers
	// packers, put back to the list
	if packer.Size() < minPackSize && s.countPacker() < maxPackers {
		debug.Log("Server.Save", "pack is not full enough (%d bytes)", packer.Size())
		s.insertPacker(packer)
		return id, nil
	}

	// else write the pack to the backend
	return id, s.savePacker(packer)
}

// SaveFrom encrypts data read from rd and stores it in a pack in the backend as type t.
func (s *Server) SaveFrom(t pack.BlobType, id backend.ID, length uint, rd io.Reader) error {
	debug.Log("Server.SaveFrom", "save id %v (%v, %d bytes)", id.Str(), t, length)
	if id == nil {
		return errors.New("id is nil")
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return err
	}

	_, err = s.Save(t, buf, id)
	if err != nil {
		return err
	}

	return nil
}

// SaveJSON serialises item as JSON and encrypts and saves it in a pack in the
// backend as type t.
func (s *Server) SaveJSON(t pack.BlobType, item interface{}) (backend.ID, error) {
	debug.Log("Server.SaveJSON", "save %v blob", t)
	buf := getBuf()[:0]
	defer freeBuf(buf)

	wr := bytes.NewBuffer(buf)

	enc := json.NewEncoder(wr)
	err := enc.Encode(item)
	if err != nil {
		return nil, fmt.Errorf("json.Encode: %v", err)
	}

	buf = wr.Bytes()
	return s.Save(t, buf, nil)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func (s *Server) SaveJSONUnpacked(t backend.Type, item interface{}) (backend.ID, error) {
	// create file
	blob, err := s.be.Create()
	if err != nil {
		return nil, err
	}
	debug.Log("Server.SaveJSONUnpacked", "create new file %p", blob)

	// hash
	hw := backend.NewHashingWriter(blob, sha256.New())

	// encrypt blob
	ewr := crypto.EncryptTo(s.key, hw)

	enc := json.NewEncoder(ewr)
	err = enc.Encode(item)
	if err != nil {
		return nil, fmt.Errorf("json.Encode: %v", err)
	}

	err = ewr.Close()
	if err != nil {
		return nil, err
	}

	// finalize blob in the backend
	sid := backend.ID(hw.Sum(nil))

	err = blob.Finalize(t, sid.String())
	if err != nil {
		return nil, err
	}

	return sid, nil
}

// Flush saves all remaining packs.
func (s *Server) Flush() error {
	s.pm.Lock()
	defer s.pm.Unlock()

	debug.Log("Server.Flush", "manually flushing %d packs", len(s.packs))

	for _, p := range s.packs {
		err := s.savePacker(p)
		if err != nil {
			return err
		}
	}
	s.packs = s.packs[:0]

	return nil
}

func (s *Server) Backend() backend.Backend {
	return s.be
}

func (s *Server) Index() *Index {
	return s.idx
}

// SetIndex instructs the server to use the given index.
func (s *Server) SetIndex(i *Index) {
	s.idx = i
}

// SaveIndex saves all new packs in the index in the backend, returned is the
// storage ID.
func (s *Server) SaveIndex() (backend.ID, error) {
	debug.Log("Server.SaveIndex", "Saving index")

	// create blob
	blob, err := s.be.Create()
	if err != nil {
		return nil, err
	}

	debug.Log("Server.SaveIndex", "create new pack %p", blob)

	// hash
	hw := backend.NewHashingWriter(blob, sha256.New())

	// encrypt blob
	ewr := crypto.EncryptTo(s.key, hw)

	err = s.idx.Encode(ewr)
	if err != nil {
		return nil, err
	}

	err = ewr.Close()
	if err != nil {
		return nil, err
	}

	// finalize blob in the backend
	sid := backend.ID(hw.Sum(nil))

	err = blob.Finalize(backend.Index, sid.String())
	if err != nil {
		return nil, err
	}

	debug.Log("Server.SaveIndex", "Saved index as %v", sid.Str())

	return sid, nil
}

// LoadIndex loads all index files from the backend and merges them with the
// current index.
func (s *Server) LoadIndex() error {
	debug.Log("Server.LoadIndex", "Loading index")
	done := make(chan struct{})
	defer close(done)

	for id := range s.be.List(backend.Index, done) {
		err := s.loadIndex(id)
		if err != nil {
			return err
		}
	}
	return nil
}

// loadIndex loads the index id and merges it with the currently used index.
func (s *Server) loadIndex(id string) error {
	debug.Log("Server.loadIndex", "Loading index %v", id[:8])
	before := len(s.idx.pack)

	rd, err := s.be.Get(backend.Index, id)
	defer rd.Close()
	if err != nil {
		return err
	}

	// decrypt
	decryptRd, err := crypto.DecryptFrom(s.key, rd)
	defer decryptRd.Close()
	if err != nil {
		return err
	}

	idx, err := DecodeIndex(decryptRd)
	if err != nil {
		debug.Log("Server.loadIndex", "error while decoding index %v: %v", id, err)
		return err
	}

	s.idx.Merge(idx)

	after := len(s.idx.pack)
	debug.Log("Server.loadIndex", "Loaded index %v, added %v blobs", id[:8], after-before)

	return nil
}

const repositoryIDSize = sha256.Size
const RepositoryVersion = 1

func (s *Server) createConfig() (err error) {
	s.Config.ChunkerPolynomial, err = chunker.RandomPolynomial()
	if err != nil {
		return err
	}

	newID := make([]byte, repositoryIDSize)
	_, err = io.ReadFull(rand.Reader, newID)
	if err != nil {
		return err
	}

	s.Config.ID = hex.EncodeToString(newID)
	s.Config.Version = RepositoryVersion

	debug.Log("Server.createConfig", "New config: %#v", s.Config)

	_, err = s.SaveJSONUnpacked(backend.Config, s.Config)
	return err
}

func (s *Server) loadConfig(cfg *Config) error {
	err := s.LoadJSONUnpacked(backend.Config, nil, cfg)
	if err != nil {
		return err
	}

	if !cfg.ChunkerPolynomial.Irreducible() {
		return errors.New("invalid chunker polynomial")
	}

	return nil
}

// SearchKey tries to find a key for which the supplied password works,
// afterwards the repository config is read and parsed.
func (s *Server) SearchKey(password string) error {
	key, err := SearchKey(s, password)
	if err != nil {
		return err
	}

	s.key = key.Master()
	s.keyName = key.Name()
	return s.loadConfig(&s.Config)
}

// CreateMasterKey creates a new key with the supplied password, afterwards the
// repository config is created.
func (s *Server) CreateMasterKey(password string) error {
	has, err := s.Test(backend.Config, "")
	if err != nil {
		return err
	}
	if has {
		return errors.New("repository master key and config already initialized")
	}

	key, err := createMasterKey(s, password)
	if err != nil {
		return err
	}

	s.key = key.Master()
	s.keyName = key.Name()
	return s.createConfig()
}

func (s *Server) Decrypt(ciphertext []byte) ([]byte, error) {
	if s.key == nil {
		return nil, errors.New("key for server not set")
	}

	return crypto.Decrypt(s.key, nil, ciphertext)
}

func (s *Server) Encrypt(ciphertext, plaintext []byte) ([]byte, error) {
	if s.key == nil {
		return nil, errors.New("key for server not set")
	}

	return crypto.Encrypt(s.key, ciphertext, plaintext)
}

func (s *Server) Key() *crypto.Key {
	return s.key
}

func (s *Server) KeyName() string {
	return s.keyName
}

// Count returns the number of blobs of a given type in the backend.
func (s *Server) Count(t backend.Type) (n uint) {
	for _ = range s.be.List(t, nil) {
		n++
	}

	return
}

// Proxy methods to backend

func (s *Server) Get(t backend.Type, name string) (io.ReadCloser, error) {
	return s.be.Get(t, name)
}

func (s *Server) List(t backend.Type, done <-chan struct{}) <-chan string {
	return s.be.List(t, done)
}

func (s *Server) Test(t backend.Type, name string) (bool, error) {
	return s.be.Test(t, name)
}

func (s *Server) Remove(t backend.Type, name string) error {
	return s.be.Remove(t, name)
}

func (s *Server) Close() error {
	return s.be.Close()
}

func (s *Server) Delete() error {
	if b, ok := s.be.(backend.Deleter); ok {
		return b.Delete()
	}

	return errors.New("Delete() called for backend that does not implement this method")
}

func (s *Server) Location() string {
	return s.be.Location()
}
