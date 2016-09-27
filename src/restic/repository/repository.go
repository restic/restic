package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"restic"

	"restic/errors"

	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
)

// Repository is used to access a repository in a backend.
type Repository struct {
	be      restic.Backend
	cfg     restic.Config
	key     *crypto.Key
	keyName string
	idx     *MasterIndex

	*packerManager
}

// New returns a new repository with backend be.
func New(be restic.Backend) *Repository {
	repo := &Repository{
		be:            be,
		idx:           NewMasterIndex(),
		packerManager: newPackerManager(be, nil),
	}

	return repo
}

// Config returns the repository configuration.
func (r *Repository) Config() restic.Config {
	return r.cfg
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (r *Repository) PrefixLength(t restic.FileType) (int, error) {
	return restic.PrefixLength(r.be, t)
}

// LoadAndDecrypt loads and decrypts data identified by t and id from the
// backend.
func (r *Repository) LoadAndDecrypt(t restic.FileType, id restic.ID) ([]byte, error) {
	debug.Log("load %v with id %v", t, id.Str())

	h := restic.Handle{Type: t, Name: id.String()}
	buf, err := backend.LoadAll(r.be, h, nil)
	if err != nil {
		debug.Log("error loading %v: %v", id.Str(), err)
		return nil, err
	}

	if t != restic.ConfigFile && !restic.Hash(buf).Equal(id) {
		return nil, errors.New("invalid data returned")
	}

	plain := make([]byte, len(buf))

	// decrypt
	n, err := r.decryptTo(plain, buf)
	if err != nil {
		return nil, err
	}

	return plain[:n], nil
}

// loadBlob tries to load and decrypt content identified by t and id from a
// pack from the backend, the result is stored in plaintextBuf, which must be
// large enough to hold the complete blob.
func (r *Repository) loadBlob(id restic.ID, t restic.BlobType, plaintextBuf []byte) (int, error) {
	debug.Log("load %v with id %v (buf %p, len %d)", t, id.Str(), plaintextBuf, len(plaintextBuf))

	// lookup plaintext size of blob
	size, err := r.idx.LookupSize(id, t)
	if err != nil {
		return 0, err
	}

	// make sure the plaintext buffer is large enough, extend otherwise
	if len(plaintextBuf) < int(size) {
		return 0, errors.Errorf("buffer is too small: %d < %d", len(plaintextBuf), size)
	}

	// lookup packs
	blobs, err := r.idx.Lookup(id, t)
	if err != nil {
		debug.Log("id %v not found in index: %v", id.Str(), err)
		return 0, err
	}

	var lastError error
	for _, blob := range blobs {
		debug.Log("id %v found: %v", id.Str(), blob)

		if blob.Type != t {
			debug.Log("blob %v has wrong block type, want %v", blob, t)
		}

		// load blob from pack
		h := restic.Handle{Type: restic.DataFile, Name: blob.PackID.String()}
		ciphertextBuf := make([]byte, blob.Length)
		n, err := r.be.Load(h, ciphertextBuf, int64(blob.Offset))
		if err != nil {
			debug.Log("error loading blob %v: %v", blob, err)
			lastError = err
			continue
		}

		if uint(n) != blob.Length {
			lastError = errors.Errorf("error loading blob %v: wrong length returned, want %d, got %d",
				id.Str(), blob.Length, uint(n))
			debug.Log("lastError: %v", lastError)
			continue
		}

		// decrypt
		n, err = r.decryptTo(plaintextBuf, ciphertextBuf)
		if err != nil {
			lastError = errors.Errorf("decrypting blob %v failed: %v", id, err)
			continue
		}
		plaintextBuf = plaintextBuf[:n]

		// check hash
		if !restic.Hash(plaintextBuf).Equal(id) {
			lastError = errors.Errorf("blob %v returned invalid hash", id)
			continue
		}

		return len(plaintextBuf), nil
	}

	if lastError != nil {
		return 0, lastError
	}

	return 0, errors.Errorf("loading blob %v from %v packs failed", id.Str(), len(blobs))
}

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (r *Repository) LoadJSONUnpacked(t restic.FileType, id restic.ID, item interface{}) (err error) {
	buf, err := r.LoadAndDecrypt(t, id)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LookupBlobSize returns the size of blob id.
func (r *Repository) LookupBlobSize(id restic.ID, tpe restic.BlobType) (uint, error) {
	return r.idx.LookupSize(id, tpe)
}

// SaveAndEncrypt encrypts data and stores it to the backend as type t. If data
// is small enough, it will be packed together with other small blobs.
func (r *Repository) SaveAndEncrypt(t restic.BlobType, data []byte, id *restic.ID) (restic.ID, error) {
	if id == nil {
		// compute plaintext hash
		hashedID := restic.Hash(data)
		id = &hashedID
	}

	debug.Log("save id %v (%v, %d bytes)", id.Str(), t, len(data))

	// get buf from the pool
	ciphertext := getBuf()
	defer freeBuf(ciphertext)

	// encrypt blob
	ciphertext, err := r.Encrypt(ciphertext, data)
	if err != nil {
		return restic.ID{}, err
	}

	// find suitable packer and add blob
	packer, err := r.findPacker(uint(len(ciphertext)))
	if err != nil {
		return restic.ID{}, err
	}

	// save ciphertext
	_, err = packer.Add(t, *id, ciphertext)
	if err != nil {
		return restic.ID{}, err
	}

	// if the pack is not full enough and there are less than maxPackers
	// packers, put back to the list
	if packer.Size() < minPackSize && r.countPacker() < maxPackers {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		r.insertPacker(packer)
		return *id, nil
	}

	// else write the pack to the backend
	return *id, r.savePacker(packer)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func (r *Repository) SaveJSONUnpacked(t restic.FileType, item interface{}) (restic.ID, error) {
	debug.Log("save new blob %v", t)
	plaintext, err := json.Marshal(item)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "json.Marshal")
	}

	return r.SaveUnpacked(t, plaintext)
}

// SaveUnpacked encrypts data and stores it in the backend. Returned is the
// storage hash.
func (r *Repository) SaveUnpacked(t restic.FileType, p []byte) (id restic.ID, err error) {
	ciphertext := make([]byte, len(p)+crypto.Extension)
	ciphertext, err = r.Encrypt(ciphertext, p)
	if err != nil {
		return restic.ID{}, err
	}

	id = restic.Hash(ciphertext)
	h := restic.Handle{Type: t, Name: id.String()}

	err = r.be.Save(h, ciphertext)
	if err != nil {
		debug.Log("error saving blob %v: %v", h, err)
		return restic.ID{}, err
	}

	debug.Log("blob %v saved", h)
	return id, nil
}

// Flush saves all remaining packs.
func (r *Repository) Flush() error {
	r.pm.Lock()
	defer r.pm.Unlock()

	debug.Log("manually flushing %d packs", len(r.packs))

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
func (r *Repository) Backend() restic.Backend {
	return r.be
}

// Index returns the currently used MasterIndex.
func (r *Repository) Index() restic.Index {
	return r.idx
}

// SetIndex instructs the repository to use the given index.
func (r *Repository) SetIndex(i restic.Index) {
	r.idx = i.(*MasterIndex)
}

// SaveIndex saves an index in the repository.
func SaveIndex(repo restic.Repository, index *Index) (restic.ID, error) {
	buf := bytes.NewBuffer(nil)

	err := index.Finalize(buf)
	if err != nil {
		return restic.ID{}, err
	}

	return repo.SaveUnpacked(restic.IndexFile, buf.Bytes())
}

// saveIndex saves all indexes in the backend.
func (r *Repository) saveIndex(indexes ...*Index) error {
	for i, idx := range indexes {
		debug.Log("Saving index %d", i)

		sid, err := SaveIndex(r, idx)
		if err != nil {
			return err
		}

		debug.Log("Saved index %d as %v", i, sid.Str())
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
	debug.Log("Loading index")

	errCh := make(chan error, 1)
	indexes := make(chan *Index)

	worker := func(id restic.ID, done <-chan struct{}) error {
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
		errCh <- FilesInParallel(r.be, restic.IndexFile, loadIndexParallelism,
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
func LoadIndex(repo restic.Repository, id restic.ID) (*Index, error) {
	idx, err := LoadIndexWithDecoder(repo, id, DecodeIndex)
	if err == nil {
		return idx, nil
	}

	if errors.Cause(err) == ErrOldIndexFormat {
		fmt.Fprintf(os.Stderr, "index %v has old format\n", id.Str())
		return LoadIndexWithDecoder(repo, id, DecodeOldIndex)
	}

	return nil, err
}

// SearchKey finds a key with the supplied password, afterwards the config is
// read and parsed. It tries at most maxKeys key files in the repo.
func (r *Repository) SearchKey(password string, maxKeys int) error {
	key, err := SearchKey(r, password, maxKeys)
	if err != nil {
		return err
	}

	r.key = key.master
	r.packerManager.key = key.master
	r.keyName = key.Name()
	r.cfg, err = restic.LoadConfig(r)
	return err
}

// Init creates a new master key with the supplied password, initializes and
// saves the repository config.
func (r *Repository) Init(password string) error {
	has, err := r.be.Test(restic.ConfigFile, "")
	if err != nil {
		return err
	}
	if has {
		return errors.New("repository master key and config already initialized")
	}

	cfg, err := restic.CreateConfig()
	if err != nil {
		return err
	}

	return r.init(password, cfg)
}

// init creates a new master key with the supplied password and uses it to save
// the config into the repo.
func (r *Repository) init(password string, cfg restic.Config) error {
	key, err := createMasterKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.packerManager.key = key.master
	r.keyName = key.Name()
	r.cfg = cfg
	_, err = r.SaveJSONUnpacked(restic.ConfigFile, cfg)
	return err
}

// decrypt authenticates and decrypts ciphertext and stores the result in
// plaintext.
func (r *Repository) decryptTo(plaintext, ciphertext []byte) (int, error) {
	if r.key == nil {
		return 0, errors.New("key for repository not set")
	}

	return crypto.Decrypt(r.key, plaintext, ciphertext)
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

func (r *Repository) list(t restic.FileType, done <-chan struct{}, out chan<- restic.ID) {
	defer close(out)
	in := r.be.List(t, done)

	var (
		// disable sending on the outCh until we received a job
		outCh chan<- restic.ID
		// enable receiving from in
		inCh = in
		id   restic.ID
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
			id, err = restic.ParseID(strID)
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
func (r *Repository) List(t restic.FileType, done <-chan struct{}) <-chan restic.ID {
	outCh := make(chan restic.ID)

	go r.list(t, done, outCh)

	return outCh
}

// ListPack returns the list of blobs saved in the pack id and the length of
// the file as stored in the backend.
func (r *Repository) ListPack(id restic.ID) ([]restic.Blob, int64, error) {
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	blobInfo, err := r.Backend().Stat(h)
	if err != nil {
		return nil, 0, err
	}

	blobs, err := pack.List(r.Key(), restic.ReaderAt(r.Backend(), h), blobInfo.Size)
	if err != nil {
		return nil, 0, err
	}

	return blobs, blobInfo.Size, nil
}

// Delete calls backend.Delete() if implemented, and returns an error
// otherwise.
func (r *Repository) Delete() error {
	if b, ok := r.be.(restic.Deleter); ok {
		return b.Delete()
	}

	return errors.New("Delete() called for backend that does not implement this method")
}

// Close closes the repository by closing the backend.
func (r *Repository) Close() error {
	return r.be.Close()
}

// LoadBlob loads a blob of type t from the repository to the buffer.
func (r *Repository) LoadBlob(t restic.BlobType, id restic.ID, buf []byte) (int, error) {
	debug.Log("load blob %v into buf %p", id.Str(), buf)
	size, err := r.idx.LookupSize(id, t)
	if err != nil {
		return 0, err
	}

	if len(buf) < int(size) {
		return 0, errors.Errorf("buffer is too small for data blob (%d < %d)", len(buf), size)
	}

	n, err := r.loadBlob(id, t, buf)
	if err != nil {
		return 0, err
	}
	buf = buf[:n]

	debug.Log("loaded %d bytes into buf %p", len(buf), buf)

	return len(buf), err
}

// SaveBlob saves a blob of type t into the repository. If id is the null id, it
// will be computed and returned.
func (r *Repository) SaveBlob(t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	var i *restic.ID
	if !id.IsNull() {
		i = &id
	}
	return r.SaveAndEncrypt(t, buf, i)
}

// LoadTree loads a tree from the repository.
func (r *Repository) LoadTree(id restic.ID) (*restic.Tree, error) {
	debug.Log("load tree %v", id.Str())

	size, err := r.idx.LookupSize(id, restic.TreeBlob)
	if err != nil {
		return nil, err
	}

	debug.Log("size is %d, create buffer", size)
	buf := make([]byte, size)

	n, err := r.loadBlob(id, restic.TreeBlob, buf)
	if err != nil {
		return nil, err
	}
	buf = buf[:n]

	t := &restic.Tree{}
	err = json.Unmarshal(buf, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// SaveTree stores a tree into the repository and returns the ID. The ID is
// checked against the index. The tree is only stored when the index does not
// contain the ID.
func (r *Repository) SaveTree(t *restic.Tree) (restic.ID, error) {
	buf, err := json.Marshal(t)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "MarshalJSON")
	}

	// append a newline so that the data is always consistent (json.Encoder
	// adds a newline after each object)
	buf = append(buf, '\n')

	id := restic.Hash(buf)
	if r.idx.Has(id, restic.TreeBlob) {
		return id, nil
	}

	_, err = r.SaveBlob(restic.TreeBlob, buf, id)
	return id, err
}
