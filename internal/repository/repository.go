package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/hashing"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
)

// Repository is used to access a repository in a backend.
type Repository struct {
	be      restic.Backend
	cfg     restic.Config
	key     *crypto.Key
	keyName string
	idx     *MasterIndex
	restic.Cache

	treePM *packerManager
	dataPM *packerManager
}

// New returns a new repository with backend be.
func New(be restic.Backend) *Repository {
	repo := &Repository{
		be:     be,
		idx:    NewMasterIndex(),
		dataPM: newPackerManager(be, nil),
		treePM: newPackerManager(be, nil),
	}

	return repo
}

// Config returns the repository configuration.
func (r *Repository) Config() restic.Config {
	return r.cfg
}

// UseCache replaces the backend with the wrapped cache.
func (r *Repository) UseCache(c restic.Cache) {
	if c == nil {
		return
	}
	debug.Log("using cache")
	r.Cache = c
	r.be = c.Wrap(r.be)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (r *Repository) PrefixLength(t restic.FileType) (int, error) {
	return restic.PrefixLength(r.be, t)
}

// LoadAndDecrypt loads and decrypts data identified by t and id from the
// backend.
func (r *Repository) LoadAndDecrypt(ctx context.Context, t restic.FileType, id restic.ID) (buf []byte, err error) {
	debug.Log("load %v with id %v", t, id)

	h := restic.Handle{Type: t, Name: id.String()}
	buf, err = backend.LoadAll(ctx, r.be, h)
	if err != nil {
		debug.Log("error loading %v: %v", h, err)
		return nil, err
	}

	if t != restic.ConfigFile && !restic.Hash(buf).Equal(id) {
		return nil, errors.Errorf("load %v: invalid data returned", h)
	}

	nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
	plaintext, err := r.key.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// sortCachedPacks moves all cached pack files to the front of blobs.
func (r *Repository) sortCachedPacks(blobs []restic.PackedBlob) []restic.PackedBlob {
	if r.Cache == nil {
		return blobs
	}

	cached := make([]restic.PackedBlob, 0, len(blobs)/2)
	noncached := make([]restic.PackedBlob, 0, len(blobs)/2)

	for _, blob := range blobs {
		if r.Cache.Has(restic.Handle{Type: restic.DataFile, Name: blob.PackID.String()}) {
			cached = append(cached, blob)
			continue
		}
		noncached = append(noncached, blob)
	}

	return append(cached, noncached...)
}

// loadBlob tries to load and decrypt content identified by t and id from a
// pack from the backend, the result is stored in plaintextBuf, which must be
// large enough to hold the complete blob.
func (r *Repository) loadBlob(ctx context.Context, id restic.ID, t restic.BlobType, plaintextBuf []byte) (int, error) {
	debug.Log("load %v with id %v (buf len %v, cap %d)", t, id, len(plaintextBuf), cap(plaintextBuf))

	// lookup packs
	blobs, found := r.idx.Lookup(id, t)
	if !found {
		debug.Log("id %v not found in index", id)
		return 0, errors.Errorf("id %v not found in repository", id)
	}

	// try cached pack files first
	blobs = r.sortCachedPacks(blobs)

	var lastError error
	for _, blob := range blobs {
		debug.Log("blob %v/%v found: %v", t, id, blob)

		if blob.Type != t {
			debug.Log("blob %v has wrong block type, want %v", blob, t)
		}

		// load blob from pack
		h := restic.Handle{Type: restic.DataFile, Name: blob.PackID.String()}

		if uint(cap(plaintextBuf)) < blob.Length {
			return 0, errors.Errorf("buffer is too small: %v < %v", cap(plaintextBuf), blob.Length)
		}

		plaintextBuf = plaintextBuf[:blob.Length]

		n, err := restic.ReadAt(ctx, r.be, h, int64(blob.Offset), plaintextBuf)
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
		nonce, ciphertext := plaintextBuf[:r.key.NonceSize()], plaintextBuf[r.key.NonceSize():]
		plaintext, err := r.key.Open(ciphertext[:0], nonce, ciphertext, nil)
		if err != nil {
			lastError = errors.Errorf("decrypting blob %v failed: %v", id, err)
			continue
		}

		// check hash
		if !restic.Hash(plaintext).Equal(id) {
			lastError = errors.Errorf("blob %v returned invalid hash", id)
			continue
		}

		// move decrypted data to the start of the provided buffer
		copy(plaintextBuf[0:], plaintext)
		return len(plaintext), nil
	}

	if lastError != nil {
		return 0, lastError
	}

	return 0, errors.Errorf("loading blob %v from %v packs failed", id.Str(), len(blobs))
}

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (r *Repository) LoadJSONUnpacked(ctx context.Context, t restic.FileType, id restic.ID, item interface{}) (err error) {
	buf, err := r.LoadAndDecrypt(ctx, t, id)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LookupBlobSize returns the size of blob id.
func (r *Repository) LookupBlobSize(id restic.ID, tpe restic.BlobType) (uint, bool) {
	return r.idx.LookupSize(id, tpe)
}

// SaveAndEncrypt encrypts data and stores it to the backend as type t. If data
// is small enough, it will be packed together with other small blobs.
func (r *Repository) SaveAndEncrypt(ctx context.Context, t restic.BlobType, data []byte, id *restic.ID) (restic.ID, error) {
	if id == nil {
		// compute plaintext hash
		hashedID := restic.Hash(data)
		id = &hashedID
	}

	debug.Log("save id %v (%v, %d bytes)", id, t, len(data))

	// get buf from the pool
	ciphertext := getBuf()

	ciphertext = ciphertext[:0]
	nonce := crypto.NewRandomNonce()
	ciphertext = append(ciphertext, nonce...)
	defer freeBuf(ciphertext)

	// encrypt blob
	ciphertext = r.key.Seal(ciphertext, nonce, data, nil)

	// find suitable packer and add blob
	var pm *packerManager

	switch t {
	case restic.TreeBlob:
		pm = r.treePM
	case restic.DataBlob:
		pm = r.dataPM
	default:
		panic(fmt.Sprintf("invalid type: %v", t))
	}

	packer, err := pm.findPacker()
	if err != nil {
		return restic.ID{}, err
	}

	// save ciphertext
	_, err = packer.Add(t, *id, ciphertext)
	if err != nil {
		return restic.ID{}, err
	}

	// if the pack is not full enough, put back to the list
	if packer.Size() < minPackSize {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		pm.insertPacker(packer)
		return *id, nil
	}

	// else write the pack to the backend
	return *id, r.savePacker(ctx, t, packer)
}

// SaveJSONUnpacked serialises item as JSON and encrypts and saves it in the
// backend as type t, without a pack. It returns the storage hash.
func (r *Repository) SaveJSONUnpacked(ctx context.Context, t restic.FileType, item interface{}) (restic.ID, error) {
	debug.Log("save new blob %v", t)
	plaintext, err := json.Marshal(item)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "json.Marshal")
	}

	return r.SaveUnpacked(ctx, t, plaintext)
}

// SaveUnpacked encrypts data and stores it in the backend. Returned is the
// storage hash.
func (r *Repository) SaveUnpacked(ctx context.Context, t restic.FileType, p []byte) (id restic.ID, err error) {
	ciphertext := restic.NewBlobBuffer(len(p))
	ciphertext = ciphertext[:0]
	nonce := crypto.NewRandomNonce()
	ciphertext = append(ciphertext, nonce...)

	ciphertext = r.key.Seal(ciphertext, nonce, p, nil)

	id = restic.Hash(ciphertext)
	h := restic.Handle{Type: t, Name: id.String()}

	err = r.be.Save(ctx, h, restic.NewByteReader(ciphertext))
	if err != nil {
		debug.Log("error saving blob %v: %v", h, err)
		return restic.ID{}, err
	}

	debug.Log("blob %v saved", h)
	return id, nil
}

// Flush saves all remaining packs.
func (r *Repository) Flush(ctx context.Context) error {
	pms := []struct {
		t  restic.BlobType
		pm *packerManager
	}{
		{restic.DataBlob, r.dataPM},
		{restic.TreeBlob, r.treePM},
	}

	for _, p := range pms {
		p.pm.pm.Lock()

		debug.Log("manually flushing %d packs", len(p.pm.packers))
		for _, packer := range p.pm.packers {
			err := r.savePacker(ctx, p.t, packer)
			if err != nil {
				p.pm.pm.Unlock()
				return err
			}
		}
		p.pm.packers = p.pm.packers[:0]
		p.pm.pm.Unlock()
	}

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
func (r *Repository) SetIndex(i restic.Index) error {
	r.idx = i.(*MasterIndex)

	ids := restic.NewIDSet()
	for _, idx := range r.idx.All() {
		id, err := idx.ID()
		if err != nil {
			debug.Log("not using index, ID() returned error %v", err)
			continue
		}
		ids.Insert(id)
	}

	return r.PrepareCache(ids)
}

// SaveIndex saves an index in the repository.
func SaveIndex(ctx context.Context, repo restic.Repository, index *Index) (restic.ID, error) {
	buf := bytes.NewBuffer(nil)

	err := index.Finalize(buf)
	if err != nil {
		return restic.ID{}, err
	}

	return repo.SaveUnpacked(ctx, restic.IndexFile, buf.Bytes())
}

// saveIndex saves all indexes in the backend.
func (r *Repository) saveIndex(ctx context.Context, indexes ...*Index) error {
	for i, idx := range indexes {
		debug.Log("Saving index %d", i)

		sid, err := SaveIndex(ctx, r, idx)
		if err != nil {
			return err
		}

		debug.Log("Saved index %d as %v", i, sid)
	}

	return nil
}

// SaveIndex saves all new indexes in the backend.
func (r *Repository) SaveIndex(ctx context.Context) error {
	return r.saveIndex(ctx, r.idx.NotFinalIndexes()...)
}

// SaveFullIndex saves all full indexes in the backend.
func (r *Repository) SaveFullIndex(ctx context.Context) error {
	return r.saveIndex(ctx, r.idx.FullIndexes()...)
}

const loadIndexParallelism = 4

// LoadIndex loads all index files from the backend in parallel and stores them
// in the master index. The first error that occurred is returned.
func (r *Repository) LoadIndex(ctx context.Context) error {
	debug.Log("Loading index")

	errCh := make(chan error, 1)
	indexes := make(chan *Index)

	worker := func(ctx context.Context, id restic.ID) error {
		idx, err := LoadIndex(ctx, r, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v, ignoring\n", err)
			return nil
		}

		select {
		case indexes <- idx:
		case <-ctx.Done():
		}

		return nil
	}

	go func() {
		defer close(indexes)
		errCh <- FilesInParallel(ctx, r.be, restic.IndexFile, loadIndexParallelism,
			ParallelWorkFuncParseID(worker))
	}()

	validIndex := restic.NewIDSet()
	for idx := range indexes {
		id, err := idx.ID()
		if err == nil {
			validIndex.Insert(id)
		}
		r.idx.Insert(idx)
	}

	err := r.PrepareCache(validIndex)
	if err != nil {
		return err
	}

	return <-errCh
}

// PrepareCache initializes the local cache. indexIDs is the list of IDs of
// index files still present in the repo.
func (r *Repository) PrepareCache(indexIDs restic.IDSet) error {
	if r.Cache == nil {
		return nil
	}

	debug.Log("prepare cache with %d index files", len(indexIDs))

	// clear old index files
	err := r.Cache.Clear(restic.IndexFile, indexIDs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error clearing index files in cache: %v\n", err)
	}

	packs := restic.NewIDSet()
	for _, idx := range r.idx.All() {
		for id := range idx.Packs() {
			packs.Insert(id)
		}
	}

	// clear old data files
	err = r.Cache.Clear(restic.DataFile, packs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error clearing data files in cache: %v\n", err)
	}

	treePacks := restic.NewIDSet()
	for _, idx := range r.idx.All() {
		for _, id := range idx.TreePacks() {
			treePacks.Insert(id)
		}
	}

	// use readahead
	debug.Log("using readahead")
	cache := r.Cache.(*cache.Cache)
	cache.PerformReadahead = func(h restic.Handle) bool {
		if h.Type != restic.DataFile {
			debug.Log("no readahead for %v, is not data file", h)
			return false
		}

		id, err := restic.ParseID(h.Name)
		if err != nil {
			debug.Log("no readahead for %v, invalid ID", h)
			return false
		}

		if treePacks.Has(id) {
			debug.Log("perform readahead for %v", h)
			return true
		}
		debug.Log("no readahead for %v, not tree file", h)
		return false
	}

	return nil
}

// LoadIndex loads the index id from backend and returns it.
func LoadIndex(ctx context.Context, repo restic.Repository, id restic.ID) (*Index, error) {
	idx, err := LoadIndexWithDecoder(ctx, repo, id, DecodeIndex)
	if err == nil {
		return idx, nil
	}

	if errors.Cause(err) == ErrOldIndexFormat {
		fmt.Fprintf(os.Stderr, "index %v has old format\n", id.Str())
		return LoadIndexWithDecoder(ctx, repo, id, DecodeOldIndex)
	}

	return nil, err
}

// SearchKey finds a key with the supplied password, afterwards the config is
// read and parsed. It tries at most maxKeys key files in the repo.
func (r *Repository) SearchKey(ctx context.Context, password string, maxKeys int, keyHint string) error {
	key, err := SearchKey(ctx, r, password, maxKeys, keyHint)
	if err != nil {
		return err
	}

	r.key = key.master
	r.dataPM.key = key.master
	r.treePM.key = key.master
	r.keyName = key.Name()
	r.cfg, err = restic.LoadConfig(ctx, r)
	if err != nil {
		return errors.Fatalf("config cannot be loaded: %v", err)
	}
	return nil
}

// Init creates a new master key with the supplied password, initializes and
// saves the repository config.
func (r *Repository) Init(ctx context.Context, password string) error {
	has, err := r.be.Test(ctx, restic.Handle{Type: restic.ConfigFile})
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

	return r.init(ctx, password, cfg)
}

// init creates a new master key with the supplied password and uses it to save
// the config into the repo.
func (r *Repository) init(ctx context.Context, password string, cfg restic.Config) error {
	key, err := createMasterKey(r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.dataPM.key = key.master
	r.treePM.key = key.master
	r.keyName = key.Name()
	r.cfg = cfg
	_, err = r.SaveJSONUnpacked(ctx, restic.ConfigFile, cfg)
	return err
}

// Key returns the current master key.
func (r *Repository) Key() *crypto.Key {
	return r.key
}

// KeyName returns the name of the current key in the backend.
func (r *Repository) KeyName() string {
	return r.keyName
}

// List runs fn for all files of type t in the repo.
func (r *Repository) List(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error {
	return r.be.List(ctx, t, func(fi restic.FileInfo) error {
		id, err := restic.ParseID(fi.Name)
		if err != nil {
			debug.Log("unable to parse %v as an ID", fi.Name)
			return nil
		}
		return fn(id, fi.Size)
	})
}

// ListPack returns the list of blobs saved in the pack id and the length of
// the file as stored in the backend.
func (r *Repository) ListPack(ctx context.Context, id restic.ID, size int64) ([]restic.Blob, int64, error) {
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	blobs, err := pack.List(r.Key(), restic.ReaderAt(r.Backend(), h), size)
	if err != nil {
		return nil, 0, err
	}

	return blobs, size, nil
}

// Delete calls backend.Delete() if implemented, and returns an error
// otherwise.
func (r *Repository) Delete(ctx context.Context) error {
	return r.be.Delete(ctx)
}

// Close closes the repository by closing the backend.
func (r *Repository) Close() error {
	return r.be.Close()
}

// LoadBlob loads a blob of type t from the repository to the buffer. buf must
// be large enough to hold the encrypted blob, since it is used as scratch
// space.
func (r *Repository) LoadBlob(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) (int, error) {
	debug.Log("load blob %v into buf (len %v, cap %v)", id, len(buf), cap(buf))
	size, found := r.idx.LookupSize(id, t)
	if !found {
		return 0, errors.Errorf("id %v not found in repository", id)
	}

	if cap(buf) < restic.CiphertextLength(int(size)) {
		return 0, errors.Errorf("buffer is too small for data blob (%d < %d)", cap(buf), restic.CiphertextLength(int(size)))
	}

	n, err := r.loadBlob(ctx, id, t, buf)
	if err != nil {
		return 0, err
	}
	buf = buf[:n]

	debug.Log("loaded %d bytes into buf %p", len(buf), buf)

	return len(buf), err
}

// SaveBlob saves a blob of type t into the repository. If id is the null id, it
// will be computed and returned.
func (r *Repository) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	var i *restic.ID
	if !id.IsNull() {
		i = &id
	}
	return r.SaveAndEncrypt(ctx, t, buf, i)
}

// LoadTree loads a tree from the repository.
func (r *Repository) LoadTree(ctx context.Context, id restic.ID) (*restic.Tree, error) {
	debug.Log("load tree %v", id)

	size, found := r.idx.LookupSize(id, restic.TreeBlob)
	if !found {
		return nil, errors.Errorf("tree %v not found in repository", id)
	}

	debug.Log("size is %d, create buffer", size)
	buf := restic.NewBlobBuffer(int(size))

	n, err := r.loadBlob(ctx, id, restic.TreeBlob, buf)
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
func (r *Repository) SaveTree(ctx context.Context, t *restic.Tree) (restic.ID, error) {
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

	_, err = r.SaveBlob(ctx, restic.TreeBlob, buf, id)
	return id, err
}

// Loader allows loading data from a backend.
type Loader interface {
	Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error
}

// DownloadAndHash is all-in-one helper to download content of the file at h to a temporary filesystem location
// and calculate ID of the contents. Returned (temporary) file is positioned at the beginning of the file;
// it is reponsibility of the caller to close and delete the file.
func DownloadAndHash(ctx context.Context, be Loader, h restic.Handle) (tmpfile *os.File, hash restic.ID, size int64, err error) {
	tmpfile, err = fs.TempFile("", "restic-temp-")
	if err != nil {
		return nil, restic.ID{}, -1, errors.Wrap(err, "TempFile")
	}

	err = be.Load(ctx, h, 0, 0, func(rd io.Reader) (ierr error) {
		_, ierr = tmpfile.Seek(0, io.SeekStart)
		if ierr == nil {
			ierr = tmpfile.Truncate(0)
		}
		if ierr != nil {
			return ierr
		}
		hrd := hashing.NewReader(rd, sha256.New())
		size, ierr = io.Copy(tmpfile, hrd)
		hash = restic.IDFromHash(hrd.Sum(nil))
		return ierr
	})
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return nil, restic.ID{}, -1, errors.Wrap(err, "Load")
	}

	_, err = tmpfile.Seek(0, io.SeekStart)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return nil, restic.ID{}, -1, errors.Wrap(err, "Seek")
	}

	return tmpfile, hash, size, err
}
