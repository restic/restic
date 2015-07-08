package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/restic/chunker"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// Repository is used to access a repository in a backend.
type Repository struct {
	be      backend.Backend
	Config  Config
	key     *crypto.Key
	keyName string
	idx     *Index

	pm    sync.Mutex
	packs []*pack.Packer
}

// New returns a new repository with backend be.
func New(be backend.Backend) *Repository {
	return &Repository{
		be:  be,
		idx: NewIndex(),
	}
}

// Find loads the list of all blobs of type t and searches for names which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func (r *Repository) Find(t backend.Type, prefix string) (string, error) {
	return backend.Find(r.be, t, prefix)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (r *Repository) PrefixLength(t backend.Type) (int, error) {
	return backend.PrefixLength(r.be, t)
}

// LoadAndDecrypt loads and decrypts data identified by t and id from the
// backend.
func (r *Repository) LoadAndDecrypt(t backend.Type, id backend.ID) ([]byte, error) {
	debug.Log("Repo.Load", "load %v with id %v", t, id.Str())

	// load blob from pack
	rd, err := r.be.Get(t, id.String())
	if err != nil {
		debug.Log("Repo.Load", "error loading %v: %v", id.Str(), err)
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
	plain, err := r.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	return plain, nil
}

// LoadBlob tries to load and decrypt content identified by t and id from a
// pack from the backend.
func (r *Repository) LoadBlob(t pack.BlobType, id backend.ID) ([]byte, error) {
	debug.Log("Repo.LoadBlob", "load %v with id %v", t, id.Str())
	// lookup pack
	packID, tpe, offset, length, err := r.idx.Lookup(id)
	if err != nil {
		debug.Log("Repo.LoadBlob", "id %v not found in index: %v", id.Str(), err)
		return nil, err
	}

	if tpe != t {
		debug.Log("Repo.LoadBlob", "wrong type returned for %v: wanted %v, got %v", id.Str(), t, tpe)
		return nil, fmt.Errorf("blob has wrong type %v (wanted: %v)", tpe, t)
	}

	debug.Log("Repo.LoadBlob", "id %v found in pack %v at offset %v (length %d)", id.Str(), packID.Str(), offset, length)

	// load blob from pack
	rd, err := r.be.GetReader(backend.Data, packID.String(), offset, length)
	if err != nil {
		debug.Log("Repo.LoadBlob", "error loading pack %v for %v: %v", packID.Str(), id.Str(), err)
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
	plain, err := r.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check hash
	if !backend.Hash(plain).Equal(id) {
		return nil, errors.New("invalid data returned")
	}

	return plain, nil
}

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (r *Repository) LoadJSONUnpacked(t backend.Type, id backend.ID, item interface{}) error {
	// load blob from backend
	rd, err := r.be.Get(t, id.String())
	if err != nil {
		return err
	}
	defer rd.Close()

	// decrypt
	decryptRd, err := crypto.DecryptFrom(r.key, rd)
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
func (r *Repository) LoadJSONPack(t pack.BlobType, id backend.ID, item interface{}) error {
	// lookup pack
	packID, _, offset, length, err := r.idx.Lookup(id)
	if err != nil {
		return err
	}

	// load blob from pack
	rd, err := r.be.GetReader(backend.Data, packID.String(), offset, length)
	if err != nil {
		return err
	}
	defer rd.Close()

	// decrypt
	decryptRd, err := crypto.DecryptFrom(r.key, rd)
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
func (r *Repository) findPacker(size uint) (*pack.Packer, error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packs) > 0 {
		debug.Log("Repo.findPacker", "searching packer for %d bytes\n", size)
		for i, p := range r.packs {
			if p.Size()+size < maxPackSize {
				debug.Log("Repo.findPacker", "found packer %v", p)
				// remove from list
				r.packs = append(r.packs[:i], r.packs[i+1:]...)
				return p, nil
			}
		}
	}

	// no suitable packer found, return new
	blob, err := r.be.Create()
	if err != nil {
		return nil, err
	}
	debug.Log("Repo.findPacker", "create new pack %p", blob)
	return pack.NewPacker(r.key, blob), nil
}

// insertPacker appends p to s.packs.
func (r *Repository) insertPacker(p *pack.Packer) {
	r.pm.Lock()
	defer r.pm.Unlock()

	r.packs = append(r.packs, p)
	debug.Log("Repo.insertPacker", "%d packers\n", len(r.packs))
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(p *pack.Packer) error {
	debug.Log("Repo.savePacker", "save packer with %d blobs\n", p.Count())
	_, err := p.Finalize()
	if err != nil {
		return err
	}

	// move file to the final location
	sid := p.ID()
	err = p.Writer().(backend.Blob).Finalize(backend.Data, sid.String())
	if err != nil {
		debug.Log("Repo.savePacker", "blob Finalize() error: %v", err)
		return err
	}

	debug.Log("Repo.savePacker", "saved as %v", sid.Str())

	// update blobs in the index
	for _, b := range p.Blobs() {
		debug.Log("Repo.savePacker", "  updating blob %v to pack %v", b.ID.Str(), sid.Str())
		r.idx.Store(b.Type, b.ID, sid, b.Offset, uint(b.Length))
	}

	return nil
}

// countPacker returns the number of open (unfinished) packers.
func (r *Repository) countPacker() int {
	r.pm.Lock()
	defer r.pm.Unlock()

	return len(r.packs)
}

// SaveAndEncrypt encrypts data and stores it to the backend as type t. If data is small
// enough, it will be packed together with other small blobs.
func (r *Repository) SaveAndEncrypt(t pack.BlobType, data []byte, id backend.ID) (backend.ID, error) {
	if id == nil {
		// compute plaintext hash
		id = backend.Hash(data)
	}

	debug.Log("Repo.Save", "save id %v (%v, %d bytes)", id.Str(), t, len(data))

	// get buf from the pool
	ciphertext := getBuf()
	defer freeBuf(ciphertext)

	// encrypt blob
	ciphertext, err := r.Encrypt(ciphertext, data)
	if err != nil {
		return nil, err
	}

	// find suitable packer and add blob
	packer, err := r.findPacker(uint(len(ciphertext)))
	if err != nil {
		return nil, err
	}

	// save ciphertext
	packer.Add(t, id, bytes.NewReader(ciphertext))

	// add this id to the index, although we don't know yet in which pack it
	// will be saved, the entry will be updated when the pack is written.
	r.idx.Store(t, id, nil, 0, 0)
	debug.Log("Repo.Save", "saving stub for %v (%v) in index", id.Str, t)

	// if the pack is not full enough and there are less than maxPackers
	// packers, put back to the list
	if packer.Size() < minPackSize && r.countPacker() < maxPackers {
		debug.Log("Repo.Save", "pack is not full enough (%d bytes)", packer.Size())
		r.insertPacker(packer)
		return id, nil
	}

	// else write the pack to the backend
	return id, r.savePacker(packer)
}

// SaveFrom encrypts data read from rd and stores it in a pack in the backend as type t.
func (r *Repository) SaveFrom(t pack.BlobType, id backend.ID, length uint, rd io.Reader) error {
	debug.Log("Repo.SaveFrom", "save id %v (%v, %d bytes)", id.Str(), t, length)
	if id == nil {
		return errors.New("id is nil")
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return err
	}

	_, err = r.SaveAndEncrypt(t, buf, id)
	if err != nil {
		return err
	}

	return nil
}

// SaveJSON serialises item as JSON and encrypts and saves it in a pack in the
// backend as type t.
func (r *Repository) SaveJSON(t pack.BlobType, item interface{}) (backend.ID, error) {
	debug.Log("Repo.SaveJSON", "save %v blob", t)
	buf := getBuf()[:0]
	defer freeBuf(buf)

	wr := bytes.NewBuffer(buf)

	enc := json.NewEncoder(wr)
	err := enc.Encode(item)
	if err != nil {
		return nil, fmt.Errorf("json.Encode: %v", err)
	}

	buf = wr.Bytes()
	return r.SaveAndEncrypt(t, buf, nil)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func (r *Repository) SaveJSONUnpacked(t backend.Type, item interface{}) (backend.ID, error) {
	// create file
	blob, err := r.be.Create()
	if err != nil {
		return nil, err
	}
	debug.Log("Repo.SaveJSONUnpacked", "create new blob %v", t)

	// hash
	hw := backend.NewHashingWriter(blob, sha256.New())

	// encrypt blob
	ewr := crypto.EncryptTo(r.key, hw)

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
		debug.Log("Repo.SaveJSONUnpacked", "error saving blob %v as %v: %v", t, sid, err)
		return nil, err
	}

	debug.Log("Repo.SaveJSONUnpacked", "new blob %v saved as %v", t, sid)

	return sid, nil
}

// Flush saves all remaining packs.
func (r *Repository) Flush() error {
	r.pm.Lock()
	defer r.pm.Unlock()

	debug.Log("Repo.Flush", "manually flushing %d packs", len(r.packs))

	for _, p := range r.packs {
		err := r.savePacker(p)
		if err != nil {
			return err
		}
	}
	r.packs = r.packs[:0]

	return nil
}

// Backend returns the backend for the repository.
func (r *Repository) Backend() backend.Backend {
	return r.be
}

// Index returns the currently loaded Index.
func (r *Repository) Index() *Index {
	return r.idx
}

// SetIndex instructs the repository to use the given index.
func (r *Repository) SetIndex(i *Index) {
	r.idx = i
}

// SaveIndex saves all new packs in the index in the backend, returned is the
// storage ID.
func (r *Repository) SaveIndex() (backend.ID, error) {
	debug.Log("Repo.SaveIndex", "Saving index")

	// create blob
	blob, err := r.be.Create()
	if err != nil {
		return nil, err
	}

	debug.Log("Repo.SaveIndex", "create new pack %p", blob)

	// hash
	hw := backend.NewHashingWriter(blob, sha256.New())

	// encrypt blob
	ewr := crypto.EncryptTo(r.key, hw)

	err = r.idx.Encode(ewr)
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

	debug.Log("Repo.SaveIndex", "Saved index as %v", sid.Str())

	return sid, nil
}

const loadIndexParallelism = 20

// LoadIndex loads all index files from the backend in parallel and merges them
// with the current index. The first error that occurred is returned.
func (r *Repository) LoadIndex() error {
	debug.Log("Repo.LoadIndex", "Loading index")

	errCh := make(chan error, 1)
	indexes := make(chan *Index)

	worker := func(id string, done <-chan struct{}) error {
		idx, err := LoadIndex(r, id)
		if err != nil {
			return err
		}

		select {
		case indexes <- idx:
		case <-done:
		}

		return nil
	}

	go func() {
		defer close(indexes)
		errCh <- FilesInParallel(r.be, backend.Index, loadIndexParallelism, worker)
	}()

	for idx := range indexes {
		r.idx.Merge(idx)
	}

	if err := <-errCh; err != nil {
		return err
	}

	return nil
}

// LoadIndex loads the index id from backend and returns it.
func LoadIndex(repo *Repository, id string) (*Index, error) {
	debug.Log("LoadIndex", "Loading index %v", id[:8])

	rd, err := repo.be.Get(backend.Index, id)
	defer rd.Close()
	if err != nil {
		return nil, err
	}

	// decrypt
	decryptRd, err := crypto.DecryptFrom(repo.key, rd)
	defer decryptRd.Close()
	if err != nil {
		return nil, err
	}

	idx, err := DecodeIndex(decryptRd)
	if err != nil {
		debug.Log("LoadIndex", "error while decoding index %v: %v", id, err)
		return nil, err
	}

	return idx, nil
}

// SearchKey finds a key with the supplied password, afterwards the config is
// read and parsed.
func (r *Repository) SearchKey(password string) error {
	key, err := SearchKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.keyName = key.Name()
	r.Config, err = LoadConfig(r)
	return err
}

// Init creates a new master key with the supplied password, initializes and
// saves the repository config.
func (r *Repository) Init(password string) error {
	has, err := r.be.Test(backend.Config, "")
	if err != nil {
		return err
	}
	if has {
		return errors.New("repository master key and config already initialized")
	}

	key, err := createMasterKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.keyName = key.Name()
	r.Config, err = CreateConfig(r)
	return err
}

// Decrypt authenticates and decrypts ciphertext and returns the plaintext.
func (r *Repository) Decrypt(ciphertext []byte) ([]byte, error) {
	if r.key == nil {
		return nil, errors.New("key for repository not set")
	}

	return crypto.Decrypt(r.key, nil, ciphertext)
}

// Encrypt encrypts and authenticates the plaintext and saves the result in
// ciphertext.
func (r *Repository) Encrypt(ciphertext, plaintext []byte) ([]byte, error) {
	if r.key == nil {
		return nil, errors.New("key for repository not set")
	}

	return crypto.Encrypt(r.key, ciphertext, plaintext)
}

// Key returns the current master key.
func (r *Repository) Key() *crypto.Key {
	return r.key
}

// KeyName returns the name of the current key in the backend.
func (r *Repository) KeyName() string {
	return r.keyName
}

// Count returns the number of blobs of a given type in the backend.
func (r *Repository) Count(t backend.Type) (n uint) {
	for _ = range r.be.List(t, nil) {
		n++
	}

	return
}

func (r *Repository) list(t backend.Type, done <-chan struct{}, out chan<- backend.ID) {
	defer close(out)
	in := r.be.List(t, done)

	var (
		// disable sending on the outCh until we received a job
		outCh chan<- backend.ID
		// enable receiving from in
		inCh = in
		id   backend.ID
		err  error
	)

	for {
		select {
		case <-done:
			return
		case strID, ok := <-inCh:
			if !ok {
				// input channel closed, we're done
				return
			}
			id, err = backend.ParseID(strID)
			if err != nil {
				// ignore invalid IDs
				continue
			}

			inCh = nil
			outCh = out
		case outCh <- id:
			outCh = nil
			inCh = in
		}
	}
}

// List returns a channel that yields all IDs of type t in the backend.
func (r *Repository) List(t backend.Type, done <-chan struct{}) <-chan backend.ID {
	outCh := make(chan backend.ID)

	go r.list(t, done, outCh)

	return outCh
}

// Delete calls backend.Delete() if implemented, and returns an error
// otherwise.
func (r *Repository) Delete() error {
	if b, ok := r.be.(backend.Deleter); ok {
		return b.Delete()
	}

	return errors.New("Delete() called for backend that does not implement this method")
}

// Close closes the repository by closing the backend.
func (r *Repository) Close() error {
	return r.be.Close()
}
