package repository

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/klauspost/compress/zstd"
	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend/dryrun"
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"

	"golang.org/x/sync/errgroup"
)

const MaxStreamBufferSize = 4 * 1024 * 1024

// Repository is used to access a repository in a backend.
type Repository struct {
	be      restic.Backend
	cfg     restic.Config
	key     *crypto.Key
	keyName string
	idx     *MasterIndex
	Cache   *cache.Cache

	opts Options

	noAutoIndexUpdate bool

	treePM *packerManager
	dataPM *packerManager

	allocEnc sync.Once
	allocDec sync.Once
	enc      *zstd.Encoder
	dec      *zstd.Decoder
}

type Options struct {
	Compression CompressionMode
}

// CompressionMode configures if data should be compressed.
type CompressionMode uint

// Constants for the different compression levels.
const (
	CompressionAuto CompressionMode = 0
	CompressionOff  CompressionMode = 1
	CompressionMax  CompressionMode = 2
)

// Set implements the method needed for pflag command flag parsing.
func (c *CompressionMode) Set(s string) error {
	switch s {
	case "auto":
		*c = CompressionAuto
	case "off":
		*c = CompressionOff
	case "max":
		*c = CompressionMax
	default:
		return fmt.Errorf("invalid compression mode %q, must be one of (auto|off|max)", s)
	}

	return nil
}

func (c *CompressionMode) String() string {
	switch *c {
	case CompressionAuto:
		return "auto"
	case CompressionOff:
		return "off"
	case CompressionMax:
		return "max"
	default:
		return "invalid"
	}

}
func (c *CompressionMode) Type() string {
	return "mode"
}

// New returns a new repository with backend be.
func New(be restic.Backend, opts Options) *Repository {
	repo := &Repository{
		be:     be,
		opts:   opts,
		idx:    NewMasterIndex(),
		dataPM: newPackerManager(be, nil),
		treePM: newPackerManager(be, nil),
	}

	return repo
}

// DisableAutoIndexUpdate deactives the automatic finalization and upload of new
// indexes once these are full
func (r *Repository) DisableAutoIndexUpdate() {
	r.noAutoIndexUpdate = true
}

// setConfig assigns the given config and updates the repository parameters accordingly
func (r *Repository) setConfig(cfg restic.Config) {
	r.cfg = cfg
	if r.cfg.Version >= 2 {
		r.idx.markCompressed()
	}
}

// Config returns the repository configuration.
func (r *Repository) Config() restic.Config {
	return r.cfg
}

// UseCache replaces the backend with the wrapped cache.
func (r *Repository) UseCache(c *cache.Cache) {
	if c == nil {
		return
	}
	debug.Log("using cache")
	r.Cache = c
	r.be = c.Wrap(r.be)
}

// SetDryRun sets the repo backend into dry-run mode.
func (r *Repository) SetDryRun() {
	r.be = dryrun.New(r.be)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (r *Repository) PrefixLength(ctx context.Context, t restic.FileType) (int, error) {
	return restic.PrefixLength(ctx, r.be, t)
}

// LoadUnpacked loads and decrypts the file with the given type and ID, using
// the supplied buffer (which must be empty). If the buffer is nil, a new
// buffer will be allocated and returned.
func (r *Repository) LoadUnpacked(ctx context.Context, buf []byte, t restic.FileType, id restic.ID) ([]byte, error) {
	if len(buf) != 0 {
		panic("buf is not empty")
	}

	debug.Log("load %v with id %v", t, id)

	if t == restic.ConfigFile {
		id = restic.ID{}
	}

	h := restic.Handle{Type: t, Name: id.String()}
	err := r.be.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		// make sure this call is idempotent, in case an error occurs
		wr := bytes.NewBuffer(buf[:0])
		_, cerr := io.Copy(wr, rd)
		if cerr != nil {
			return cerr
		}
		buf = wr.Bytes()
		return nil
	})

	if err != nil {
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
	if t != restic.ConfigFile {
		return r.decompressUnpacked(plaintext)
	}

	return plaintext, nil
}

type haver interface {
	Has(restic.Handle) bool
}

// sortCachedPacksFirst moves all cached pack files to the front of blobs.
func sortCachedPacksFirst(cache haver, blobs []restic.PackedBlob) {
	if cache == nil {
		return
	}

	// no need to sort a list with one element
	if len(blobs) == 1 {
		return
	}

	cached := blobs[:0]
	noncached := make([]restic.PackedBlob, 0, len(blobs)/2)

	for _, blob := range blobs {
		if cache.Has(restic.Handle{Type: restic.PackFile, Name: blob.PackID.String()}) {
			cached = append(cached, blob)
			continue
		}
		noncached = append(noncached, blob)
	}

	copy(blobs[len(cached):], noncached)
}

// LoadBlob loads a blob of type t from the repository.
// It may use all of buf[:cap(buf)] as scratch space.
func (r *Repository) LoadBlob(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error) {
	debug.Log("load %v with id %v (buf len %v, cap %d)", t, id, len(buf), cap(buf))

	// lookup packs
	blobs := r.idx.Lookup(restic.BlobHandle{ID: id, Type: t})
	if len(blobs) == 0 {
		debug.Log("id %v not found in index", id)
		return nil, errors.Errorf("id %v not found in repository", id)
	}

	// try cached pack files first
	sortCachedPacksFirst(r.Cache, blobs)

	var lastError error
	for _, blob := range blobs {
		debug.Log("blob %v/%v found: %v", t, id, blob)

		if blob.Type != t {
			debug.Log("blob %v has wrong block type, want %v", blob, t)
		}

		// load blob from pack
		bt := t
		if r.idx.IsMixedPack(blob.PackID) {
			bt = restic.InvalidBlob
		}
		h := restic.Handle{Type: restic.PackFile,
			Name: blob.PackID.String(), ContainedBlobType: bt}

		switch {
		case cap(buf) < int(blob.Length):
			buf = make([]byte, blob.Length)
		case len(buf) != int(blob.Length):
			buf = buf[:blob.Length]
		}

		n, err := restic.ReadAt(ctx, r.be, h, int64(blob.Offset), buf)
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
		nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
		plaintext, err := r.key.Open(ciphertext[:0], nonce, ciphertext, nil)
		if err != nil {
			lastError = errors.Errorf("decrypting blob %v failed: %v", id, err)
			continue
		}

		if blob.IsCompressed() {
			plaintext, err = r.getZstdDecoder().DecodeAll(plaintext, make([]byte, 0, blob.DataLength()))
			if err != nil {
				lastError = errors.Errorf("decompressing blob %v failed: %v", id, err)
				continue
			}
		}

		// check hash
		if !restic.Hash(plaintext).Equal(id) {
			lastError = errors.Errorf("blob %v returned invalid hash", id)
			continue
		}

		if len(plaintext) > cap(buf) {
			return plaintext, nil
		}
		// move decrypted data to the start of the buffer
		copy(buf, plaintext)
		return buf[:len(plaintext)], nil
	}

	if lastError != nil {
		return nil, lastError
	}

	return nil, errors.Errorf("loading blob %v from %v packs failed", id.Str(), len(blobs))
}

// LoadJSONUnpacked decrypts the data and afterwards calls json.Unmarshal on
// the item.
func (r *Repository) LoadJSONUnpacked(ctx context.Context, t restic.FileType, id restic.ID, item interface{}) (err error) {
	buf, err := r.LoadUnpacked(ctx, nil, t, id)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, item)
}

// LookupBlobSize returns the size of blob id.
func (r *Repository) LookupBlobSize(id restic.ID, tpe restic.BlobType) (uint, bool) {
	return r.idx.LookupSize(restic.BlobHandle{ID: id, Type: tpe})
}

func (r *Repository) getZstdEncoder() *zstd.Encoder {
	r.allocEnc.Do(func() {
		level := zstd.SpeedDefault
		if r.opts.Compression == CompressionMax {
			level = zstd.SpeedBestCompression
		}

		opts := []zstd.EOption{
			// Set the compression level configured.
			zstd.WithEncoderLevel(level),
			// Disable CRC, we have enough checks in place, makes the
			// compressed data four bytes shorter.
			zstd.WithEncoderCRC(false),
			// Set a window of 512kbyte, so we have good lookbehind for usual
			// blob sizes.
			zstd.WithWindowSize(512 * 1024),
		}

		enc, err := zstd.NewWriter(nil, opts...)
		if err != nil {
			panic(err)
		}
		r.enc = enc
	})
	return r.enc
}

func (r *Repository) getZstdDecoder() *zstd.Decoder {
	r.allocDec.Do(func() {
		opts := []zstd.DOption{
			// Use all available cores.
			zstd.WithDecoderConcurrency(0),
			// Limit the maximum decompressed memory. Set to a very high,
			// conservative value.
			zstd.WithDecoderMaxMemory(16 * 1024 * 1024 * 1024),
		}

		dec, err := zstd.NewReader(nil, opts...)
		if err != nil {
			panic(err)
		}
		r.dec = dec
	})
	return r.dec
}

// saveAndEncrypt encrypts data and stores it to the backend as type t. If data
// is small enough, it will be packed together with other small blobs.
// The caller must ensure that the id matches the data.
func (r *Repository) saveAndEncrypt(ctx context.Context, t restic.BlobType, data []byte, id restic.ID) error {
	debug.Log("save id %v (%v, %d bytes)", id, t, len(data))

	uncompressedLength := 0
	if r.cfg.Version > 1 {

		// we have a repo v2, so compression is available. if the user opts to
		// not compress, we won't compress any data, but everything else is
		// compressed.
		if r.opts.Compression != CompressionOff || t != restic.DataBlob {
			uncompressedLength = len(data)
			data = r.getZstdEncoder().EncodeAll(data, nil)
		}
	}

	nonce := crypto.NewRandomNonce()

	ciphertext := make([]byte, 0, restic.CiphertextLength(len(data)))
	ciphertext = append(ciphertext, nonce...)

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
		return err
	}

	// save ciphertext
	_, err = packer.Add(t, id, ciphertext, uncompressedLength)
	if err != nil {
		return err
	}

	// if the pack is not full enough, put back to the list
	if packer.Size() < minPackSize {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		pm.insertPacker(packer)
		return nil
	}

	// else write the pack to the backend
	return r.savePacker(ctx, t, packer)
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

func (r *Repository) compressUnpacked(p []byte) ([]byte, error) {
	// compression is only available starting from version 2
	if r.cfg.Version < 2 {
		return p, nil
	}

	// version byte
	out := []byte{2}
	out = r.getZstdEncoder().EncodeAll(p, out)
	return out, nil
}

func (r *Repository) decompressUnpacked(p []byte) ([]byte, error) {
	// compression is only available starting from version 2
	if r.cfg.Version < 2 {
		return p, nil
	}

	if len(p) == 0 {
		// too short for version header
		return p, nil
	}
	if p[0] == '[' || p[0] == '{' {
		// probably raw JSON
		return p, nil
	}
	// version
	if p[0] != 2 {
		return nil, errors.New("not supported encoding format")
	}

	return r.getZstdDecoder().DecodeAll(p[1:], nil)
}

// SaveUnpacked encrypts data and stores it in the backend. Returned is the
// storage hash.
func (r *Repository) SaveUnpacked(ctx context.Context, t restic.FileType, p []byte) (id restic.ID, err error) {
	if t != restic.ConfigFile {
		p, err = r.compressUnpacked(p)
		if err != nil {
			return restic.ID{}, err
		}
	}

	ciphertext := restic.NewBlobBuffer(len(p))
	ciphertext = ciphertext[:0]
	nonce := crypto.NewRandomNonce()
	ciphertext = append(ciphertext, nonce...)

	ciphertext = r.key.Seal(ciphertext, nonce, p, nil)

	if t == restic.ConfigFile {
		id = restic.ID{}
	} else {
		id = restic.Hash(ciphertext)
	}
	h := restic.Handle{Type: t, Name: id.String()}

	err = r.be.Save(ctx, h, restic.NewByteReader(ciphertext, r.be.Hasher()))
	if err != nil {
		debug.Log("error saving blob %v: %v", h, err)
		return restic.ID{}, err
	}

	debug.Log("blob %v saved", h)
	return id, nil
}

// Flush saves all remaining packs and the index
func (r *Repository) Flush(ctx context.Context) error {
	if err := r.FlushPacks(ctx); err != nil {
		return err
	}

	// Save index after flushing only if noAutoIndexUpdate is not set
	if r.noAutoIndexUpdate {
		return nil
	}
	return r.SaveIndex(ctx)
}

// FlushPacks saves all remaining packs.
func (r *Repository) FlushPacks(ctx context.Context) error {
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
func (r *Repository) Index() restic.MasterIndex {
	return r.idx
}

// SetIndex instructs the repository to use the given index.
func (r *Repository) SetIndex(i restic.MasterIndex) error {
	r.idx = i.(*MasterIndex)
	return r.PrepareCache()
}

// SaveIndex saves an index in the repository.
func SaveIndex(ctx context.Context, repo restic.Repository, index *Index) (restic.ID, error) {
	buf := bytes.NewBuffer(nil)

	err := index.Encode(buf)
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

	return r.idx.MergeFinalIndexes()
}

// SaveIndex saves all new indexes in the backend.
func (r *Repository) SaveIndex(ctx context.Context) error {
	return r.saveIndex(ctx, r.idx.FinalizeNotFinalIndexes()...)
}

// SaveFullIndex saves all full indexes in the backend.
func (r *Repository) SaveFullIndex(ctx context.Context) error {
	return r.saveIndex(ctx, r.idx.FinalizeFullIndexes()...)
}

// LoadIndex loads all index files from the backend in parallel and stores them
// in the master index. The first error that occurred is returned.
func (r *Repository) LoadIndex(ctx context.Context) error {
	debug.Log("Loading index")

	err := ForAllIndexes(ctx, r, func(id restic.ID, idx *Index, oldFormat bool, err error) error {
		if err != nil {
			return err
		}

		_, err = idx.IDs()
		if err != nil {
			return err
		}

		r.idx.Insert(idx)
		return nil
	})

	if err != nil {
		return errors.Fatal(err.Error())
	}

	err = r.idx.MergeFinalIndexes()
	if err != nil {
		return err
	}

	if r.cfg.Version < 2 {
		// sanity check
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		for blob := range r.idx.Each(ctx) {
			if blob.IsCompressed() {
				return errors.Fatal("index uses feature not supported by repository version 1")
			}
		}
	}

	// remove index files from the cache which have been removed in the repo
	return r.PrepareCache()
}

const listPackParallelism = 10

// CreateIndexFromPacks creates a new index by reading all given pack files (with sizes).
// The index is added to the MasterIndex but not marked as finalized.
// Returned is the list of pack files which could not be read.
func (r *Repository) CreateIndexFromPacks(ctx context.Context, packsize map[restic.ID]int64, p *progress.Counter) (invalid restic.IDs, err error) {
	var m sync.Mutex

	debug.Log("Loading index from pack files")

	// track spawned goroutines using wg, create a new context which is
	// cancelled as soon as an error occurs.
	wg, ctx := errgroup.WithContext(ctx)

	type FileInfo struct {
		restic.ID
		Size int64
	}
	ch := make(chan FileInfo)

	// send list of pack files through ch, which is closed afterwards
	wg.Go(func() error {
		defer close(ch)
		for id, size := range packsize {
			select {
			case <-ctx.Done():
				return nil
			case ch <- FileInfo{id, size}:
			}
		}
		return nil
	})

	idx := NewIndex()
	// a worker receives an pack ID from ch, reads the pack contents, and adds them to idx
	worker := func() error {
		for fi := range ch {
			entries, _, err := r.ListPack(ctx, fi.ID, fi.Size)
			if err != nil {
				debug.Log("unable to list pack file %v", fi.ID.Str())
				m.Lock()
				invalid = append(invalid, fi.ID)
				m.Unlock()
			}
			idx.StorePack(fi.ID, entries)
			p.Add(1)
		}

		return nil
	}

	// run workers on ch
	wg.Go(func() error {
		return RunWorkers(listPackParallelism, worker)
	})

	err = wg.Wait()
	if err != nil {
		return invalid, errors.Fatal(err.Error())
	}

	// Add idx to MasterIndex
	r.idx.Insert(idx)

	return invalid, nil
}

// PrepareCache initializes the local cache. indexIDs is the list of IDs of
// index files still present in the repo.
func (r *Repository) PrepareCache() error {
	if r.Cache == nil {
		return nil
	}

	indexIDs := r.idx.IDs()
	debug.Log("prepare cache with %d index files", len(indexIDs))

	// clear old index files
	err := r.Cache.Clear(restic.IndexFile, indexIDs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error clearing index files in cache: %v\n", err)
	}

	packs := r.idx.Packs(restic.NewIDSet())

	// clear old packs
	err = r.Cache.Clear(restic.PackFile, packs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error clearing pack files in cache: %v\n", err)
	}

	return nil
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
	cfg, err := restic.LoadConfig(ctx, r)
	if err == crypto.ErrUnauthenticated {
		return errors.Fatalf("config or key %v is damaged: %v", key.Name(), err)
	} else if err != nil {
		return errors.Fatalf("config cannot be loaded: %v", err)
	}

	r.setConfig(cfg)
	return nil
}

// Init creates a new master key with the supplied password, initializes and
// saves the repository config.
func (r *Repository) Init(ctx context.Context, version uint, password string, chunkerPolynomial *chunker.Pol) error {
	if version > restic.MaxRepoVersion {
		return fmt.Errorf("repo version %v too high", version)
	}

	if version < restic.MinRepoVersion {
		return fmt.Errorf("repo version %v too low", version)
	}

	has, err := r.be.Test(ctx, restic.Handle{Type: restic.ConfigFile})
	if err != nil {
		return err
	}
	if has {
		return errors.New("repository master key and config already initialized")
	}

	cfg, err := restic.CreateConfig(version)
	if err != nil {
		return err
	}
	if chunkerPolynomial != nil {
		cfg.ChunkerPolynomial = *chunkerPolynomial
	}

	return r.init(ctx, password, cfg)
}

// init creates a new master key with the supplied password and uses it to save
// the config into the repo.
func (r *Repository) init(ctx context.Context, password string, cfg restic.Config) error {
	key, err := createMasterKey(ctx, r, password)
	if err != nil {
		return err
	}

	r.key = key.master
	r.dataPM.key = key.master
	r.treePM.key = key.master
	r.keyName = key.Name()
	r.setConfig(cfg)
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
// the the pack header.
func (r *Repository) ListPack(ctx context.Context, id restic.ID, size int64) ([]restic.Blob, uint32, error) {
	h := restic.Handle{Type: restic.PackFile, Name: id.String()}

	return pack.List(r.Key(), restic.ReaderAt(ctx, r.Backend(), h), size)
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

// SaveBlob saves a blob of type t into the repository.
// It takes care that no duplicates are saved; this can be overwritten
// by setting storeDuplicate to true.
// If id is the null id, it will be computed and returned.
// Also returns if the blob was already known before
func (r *Repository) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, err error) {

	// compute plaintext hash if not already set
	if id.IsNull() {
		newID = restic.Hash(buf)
	} else {
		newID = id
	}

	// first try to add to pending blobs; if not successful, this blob is already known
	known = !r.idx.addPending(restic.BlobHandle{ID: newID, Type: t})

	// only save when needed or explicitly told
	if !known || storeDuplicate {
		err = r.saveAndEncrypt(ctx, t, buf, newID)
	}

	return newID, known, err
}

// LoadTree loads a tree from the repository.
func (r *Repository) LoadTree(ctx context.Context, id restic.ID) (*restic.Tree, error) {
	debug.Log("load tree %v", id)

	buf, err := r.LoadBlob(ctx, restic.TreeBlob, id, nil)
	if err != nil {
		return nil, err
	}

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

	id, _, err := r.SaveBlob(ctx, restic.TreeBlob, buf, restic.ID{}, false)
	return id, err
}

// Loader allows loading data from a backend.
type Loader interface {
	Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error
}

type BackendLoadFn func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error

// StreamPack loads the listed blobs from the specified pack file. The plaintext blob is passed to
// the handleBlobFn callback or an error if decryption failed or the blob hash does not match. In
// case of download errors handleBlobFn might be called multiple times for the same blob. If the
// callback returns an error, then StreamPack will abort and not retry it.
func StreamPack(ctx context.Context, beLoad BackendLoadFn, key *crypto.Key, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	if len(blobs) == 0 {
		// nothing to do
		return nil
	}

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})
	h := restic.Handle{Type: restic.PackFile, Name: packID.String(), ContainedBlobType: restic.DataBlob}

	dataStart := blobs[0].Offset
	dataEnd := blobs[len(blobs)-1].Offset + blobs[len(blobs)-1].Length

	debug.Log("streaming pack %v (%d to %d bytes), blobs: %v", packID, dataStart, dataEnd, len(blobs))

	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(dec)
	}
	defer dec.Close()

	ctx, cancel := context.WithCancel(ctx)
	// stream blobs in pack
	err = beLoad(ctx, h, int(dataEnd-dataStart), int64(dataStart), func(rd io.Reader) error {
		// prevent callbacks after cancelation
		if ctx.Err() != nil {
			return ctx.Err()
		}
		bufferSize := int(dataEnd - dataStart)
		if bufferSize > MaxStreamBufferSize {
			bufferSize = MaxStreamBufferSize
		}
		// create reader here to allow reusing the buffered reader from checker.checkData
		bufRd := bufio.NewReaderSize(rd, bufferSize)
		currentBlobEnd := dataStart
		var buf []byte
		var decode []byte
		for _, entry := range blobs {
			skipBytes := int(entry.Offset - currentBlobEnd)
			if skipBytes < 0 {
				return errors.Errorf("overlapping blobs in pack %v", packID)
			}

			_, err := bufRd.Discard(skipBytes)
			if err != nil {
				return err
			}

			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			debug.Log("  process blob %v, skipped %d, %v", h, skipBytes, entry)

			if uint(cap(buf)) < entry.Length {
				buf = make([]byte, entry.Length)
			}
			buf = buf[:entry.Length]

			n, err := io.ReadFull(bufRd, buf)
			if err != nil {
				debug.Log("    read error %v", err)
				return errors.Wrap(err, "ReadFull")
			}

			if n != len(buf) {
				return errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
					h, packID.Str(), len(buf), n)
			}
			currentBlobEnd = entry.Offset + entry.Length

			if int(entry.Length) <= key.NonceSize() {
				debug.Log("%v", blobs)
				return errors.Errorf("invalid blob length %v", entry)
			}

			// decryption errors are likely permanent, give the caller a chance to skip them
			nonce, ciphertext := buf[:key.NonceSize()], buf[key.NonceSize():]
			plaintext, err := key.Open(ciphertext[:0], nonce, ciphertext, nil)
			if err == nil && entry.IsCompressed() {
				// DecodeAll will allocate a slice if it is not large enough since it
				// knows the decompressed size (because we're using EncodeAll)
				decode, err = dec.DecodeAll(plaintext, decode[:0])
				plaintext = decode
				if err != nil {
					err = errors.Errorf("decompressing blob %v failed: %v", h, err)
				}
			}
			if err == nil {
				id := restic.Hash(plaintext)
				if !id.Equal(entry.ID) {
					debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
						h.Type, h.ID, packID.Str(), id)
					err = errors.Errorf("read blob %v from %v: wrong data returned, hash is %v",
						h, packID.Str(), id)
				}
			}

			err = handleBlobFn(entry.BlobHandle, plaintext, err)
			if err != nil {
				cancel()
				return backoff.Permanent(err)
			}
		}
		return nil
	})
	return errors.Wrap(err, "StreamPack")
}
