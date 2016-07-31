package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
)

// Repository is used to access a repository in a backend.
type Repository struct {
	be      backend.Backend
	Config  Config
	key     *crypto.Key
	keyName string
	idx     *MasterIndex

	*packerManager
}

// New returns a new repository with backend be.
func New(be backend.Backend) *Repository {
	repo := &Repository{
		be:            be,
		idx:           NewMasterIndex(),
		packerManager: newPackerManager(be, nil),
	}

	return repo
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

	h := backend.Handle{Type: t, Name: id.String()}
	buf, err := backend.LoadAll(r.be, h, nil)
	if err != nil {
		debug.Log("Repo.Load", "error loading %v: %v", id.Str(), err)
		return nil, err
	}

	if t != backend.Config && !backend.Hash(buf).Equal(id) {
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
// pack from the backend, the result is stored in plaintextBuf, which must be
// large enough to hold the complete blob.
func (r *Repository) LoadBlob(t pack.BlobType, id backend.ID, plaintextBuf []byte) ([]byte, error) {
	debug.Log("Repo.LoadBlob", "load %v with id %v", t, id.Str())
	// lookup pack
	blob, err := r.idx.Lookup(id)
	if err != nil {
		debug.Log("Repo.LoadBlob", "id %v not found in index: %v", id.Str(), err)
		return nil, err
	}

	plaintextBufSize := uint(cap(plaintextBuf))
	if blob.PlaintextLength() > plaintextBufSize {
		debug.Log("Repo.LoadBlob", "need to expand buffer: want %d bytes, got %d",
			blob.PlaintextLength(), plaintextBufSize)
		plaintextBuf = make([]byte, blob.PlaintextLength())
	}

	if blob.Type != t {
		debug.Log("Repo.LoadBlob", "wrong type returned for %v: wanted %v, got %v", id.Str(), t, blob.Type)
		return nil, fmt.Errorf("blob has wrong type %v (wanted: %v)", blob.Type, t)
	}

	debug.Log("Repo.LoadBlob", "id %v found: %v", id.Str(), blob)

	// load blob from pack
	h := backend.Handle{Type: backend.Data, Name: blob.PackID.String()}
	ciphertextBuf := make([]byte, blob.Length)
	n, err := r.be.Load(h, ciphertextBuf, int64(blob.Offset))
	if err != nil {
		debug.Log("Repo.LoadBlob", "error loading blob %v: %v", blob, err)
		return nil, err
	}

	if uint(n) != blob.Length {
		debug.Log("Repo.LoadBlob", "error loading blob %v: wrong length returned, want %d, got %d",
			blob.Length, uint(n))
		return nil, errors.New("wrong length returned")
	}

	// decrypt
	plaintextBuf, err = r.decryptTo(plaintextBuf, ciphertextBuf)
	if err != nil {
		return nil, err
	}

	// check hash
	if !backend.Hash(plaintextBuf).Equal(id) {
		return nil, errors.New("invalid data returned")
	}

	return plaintextBuf, nil
}

// closeOrErr calls cl.Close() and sets err to the returned error value if
// itself is not yet set.
func closeOrErr(cl io.Closer, err *error) {
	e := cl.Close()
	if *err != nil {
		return
	}
	*err = e
}

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (r *Repository) LoadJSONUnpacked(t backend.Type, id backend.ID, item interface{}) (err error) {
	buf, err := r.LoadAndDecrypt(t, id)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LoadJSONPack calls LoadBlob() to load a blob from the backend, decrypt the
// data and afterwards call json.Unmarshal on the item.
func (r *Repository) LoadJSONPack(t pack.BlobType, id backend.ID, item interface{}) (err error) {
	buf, err := r.LoadBlob(t, id, nil)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LookupBlobSize returns the size of blob id.
func (r *Repository) LookupBlobSize(id backend.ID) (uint, error) {
	return r.idx.LookupSize(id)
}

// SaveAndEncrypt encrypts data and stores it to the backend as type t. If data
// is small enough, it will be packed together with other small blobs.
func (r *Repository) SaveAndEncrypt(t pack.BlobType, data []byte, id *backend.ID) (backend.ID, error) {
	if id == nil {
		// compute plaintext hash
		hashedID := backend.Hash(data)
		id = &hashedID
	}

	debug.Log("Repo.Save", "save id %v (%v, %d bytes)", id.Str(), t, len(data))

	// get buf from the pool
	ciphertext := getBuf()
	defer freeBuf(ciphertext)

	// encrypt blob
	ciphertext, err := r.Encrypt(ciphertext, data)
	if err != nil {
		return backend.ID{}, err
	}

	// find suitable packer and add blob
	packer, err := r.findPacker(uint(len(ciphertext)))
	if err != nil {
		return backend.ID{}, err
	}

	// save ciphertext
	_, err = packer.Add(t, *id, ciphertext)
	if err != nil {
		return backend.ID{}, err
	}

	// if the pack is not full enough and there are less than maxPackers
	// packers, put back to the list
	if packer.Size() < minPackSize && r.countPacker() < maxPackers {
		debug.Log("Repo.Save", "pack is not full enough (%d bytes)", packer.Size())
		r.insertPacker(packer)
		return *id, nil
	}

	// else write the pack to the backend
	return *id, r.savePacker(packer)
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
		return backend.ID{}, fmt.Errorf("json.Encode: %v", err)
	}

	buf = wr.Bytes()
	return r.SaveAndEncrypt(t, buf, nil)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func (r *Repository) SaveJSONUnpacked(t backend.Type, item interface{}) (backend.ID, error) {
	debug.Log("Repo.SaveJSONUnpacked", "save new blob %v", t)
	plaintext, err := json.Marshal(item)
	if err != nil {
		return backend.ID{}, fmt.Errorf("json.Encode: %v", err)
	}

	return r.SaveUnpacked(t, plaintext)
}

// SaveUnpacked encrypts data and stores it in the backend. Returned is the
// storage hash.
func (r *Repository) SaveUnpacked(t backend.Type, p []byte) (id backend.ID, err error) {
	ciphertext := make([]byte, len(p)+crypto.Extension)
	ciphertext, err = r.Encrypt(ciphertext, p)
	if err != nil {
		return backend.ID{}, err
	}

	id = backend.Hash(ciphertext)
	h := backend.Handle{Type: t, Name: id.String()}

	err = r.be.Save(h, ciphertext)
	if err != nil {
		debug.Log("Repo.SaveJSONUnpacked", "error saving blob %v: %v", h, err)
		return backend.ID{}, err
	}

	debug.Log("Repo.SaveJSONUnpacked", "blob %v saved", h)
	return id, nil
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

// Index returns the currently used MasterIndex.
func (r *Repository) Index() *MasterIndex {
	return r.idx
}

// SetIndex instructs the repository to use the given index.
func (r *Repository) SetIndex(i *MasterIndex) {
	r.idx = i
}

// SaveIndex saves an index in the repository.
func SaveIndex(repo *Repository, index *Index) (backend.ID, error) {
	buf := bytes.NewBuffer(nil)

	err := index.Finalize(buf)
	if err != nil {
		return backend.ID{}, err
	}

	return repo.SaveUnpacked(backend.Index, buf.Bytes())
}

// saveIndex saves all indexes in the backend.
func (r *Repository) saveIndex(indexes ...*Index) error {
	for i, idx := range indexes {
		debug.Log("Repo.SaveIndex", "Saving index %d", i)

		sid, err := SaveIndex(r, idx)
		if err != nil {
			return err
		}

		debug.Log("Repo.SaveIndex", "Saved index %d as %v", i, sid.Str())
	}

	return nil
}

// SaveIndex saves all new indexes in the backend.
func (r *Repository) SaveIndex() error {
	return r.saveIndex(r.idx.NotFinalIndexes()...)
}

// SaveFullIndex saves all full indexes in the backend.
func (r *Repository) SaveFullIndex() error {
	return r.saveIndex(r.idx.FullIndexes()...)
}

const loadIndexParallelism = 20

// LoadIndex loads all index files from the backend in parallel and stores them
// in the master index. The first error that occurred is returned.
func (r *Repository) LoadIndex() error {
	debug.Log("Repo.LoadIndex", "Loading index")

	errCh := make(chan error, 1)
	indexes := make(chan *Index)

	worker := func(id backend.ID, done <-chan struct{}) error {
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
		errCh <- FilesInParallel(r.be, backend.Index, loadIndexParallelism,
			ParallelWorkFuncParseID(worker))
	}()

	for idx := range indexes {
		r.idx.Insert(idx)
	}

	if err := <-errCh; err != nil {
		return err
	}

	return nil
}

// LoadIndex loads the index id from backend and returns it.
func LoadIndex(repo *Repository, id backend.ID) (*Index, error) {
	idx, err := LoadIndexWithDecoder(repo, id, DecodeIndex)
	if err == nil {
		return idx, nil
	}

	if err == ErrOldIndexFormat {
		fmt.Fprintf(os.Stderr, "index %v has old format\n", id.Str())
		return LoadIndexWithDecoder(repo, id, DecodeOldIndex)
	}

	return nil, err
}

// SearchKey finds a key with the supplied password, afterwards the config is
// read and parsed.
func (r *Repository) SearchKey(password string) error {
	key, err := SearchKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.packerManager.key = key.master
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

	cfg, err := CreateConfig()
	if err != nil {
		return err
	}

	return r.init(password, cfg)
}

// init creates a new master key with the supplied password and uses it to save
// the config into the repo.
func (r *Repository) init(password string, cfg Config) error {
	key, err := createMasterKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.packerManager.key = key.master
	r.keyName = key.Name()
	r.Config = cfg
	_, err = r.SaveJSONUnpacked(backend.Config, cfg)
	return err
}

// Decrypt authenticates and decrypts ciphertext and returns the plaintext.
func (r *Repository) Decrypt(ciphertext []byte) ([]byte, error) {
	return r.decryptTo(nil, ciphertext)
}

// decrypt authenticates and decrypts ciphertext and stores the result in
// plaintext.
func (r *Repository) decryptTo(plaintext, ciphertext []byte) ([]byte, error) {
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

// ListPack returns the list of blobs saved in the pack id.
func (r *Repository) ListPack(id backend.ID) ([]pack.Blob, error) {
	h := backend.Handle{Type: backend.Data, Name: id.String()}
	rd := backend.NewReadSeeker(r.Backend(), h)

	unpacker, err := pack.NewUnpacker(r.Key(), rd)
	if err != nil {
		return nil, err
	}

	return unpacker.Entries, nil
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
