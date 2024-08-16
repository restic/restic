package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/backend/dryrun"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"

	"golang.org/x/sync/errgroup"
)

const MinPackSize = 4 * 1024 * 1024
const DefaultPackSize = 16 * 1024 * 1024
const MaxPackSize = 128 * 1024 * 1024

// Repository is used to access a repository in a backend.
type Repository struct {
	be    backend.Backend
	cfg   restic.Config
	key   *crypto.Key
	keyID restic.ID
	idx   *index.MasterIndex
	Cache *cache.Cache

	opts Options

	packerWg *errgroup.Group
	uploader *packerUploader
	treePM   *packerManager
	dataPM   *packerManager

	allocEnc sync.Once
	allocDec sync.Once
	enc      *zstd.Encoder
	dec      *zstd.Decoder
}

type Options struct {
	Compression   CompressionMode
	PackSize      uint
	NoExtraVerify bool
}

// CompressionMode configures if data should be compressed.
type CompressionMode uint

// Constants for the different compression levels.
const (
	CompressionAuto    CompressionMode = 0
	CompressionOff     CompressionMode = 1
	CompressionMax     CompressionMode = 2
	CompressionInvalid CompressionMode = 3
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
		*c = CompressionInvalid
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
func New(be backend.Backend, opts Options) (*Repository, error) {
	if opts.Compression == CompressionInvalid {
		return nil, errors.New("invalid compression mode")
	}

	if opts.PackSize == 0 {
		opts.PackSize = DefaultPackSize
	}
	if opts.PackSize > MaxPackSize {
		return nil, fmt.Errorf("pack size larger than limit of %v MiB", MaxPackSize/1024/1024)
	} else if opts.PackSize < MinPackSize {
		return nil, fmt.Errorf("pack size smaller than minimum of %v MiB", MinPackSize/1024/1024)
	}

	repo := &Repository{
		be:   be,
		opts: opts,
		idx:  index.NewMasterIndex(),
	}

	return repo, nil
}

// setConfig assigns the given config and updates the repository parameters accordingly
func (r *Repository) setConfig(cfg restic.Config) {
	r.cfg = cfg
}

// Config returns the repository configuration.
func (r *Repository) Config() restic.Config {
	return r.cfg
}

// packSize return the target size of a pack file when uploading
func (r *Repository) packSize() uint {
	return r.opts.PackSize
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

// LoadUnpacked loads and decrypts the file with the given type and ID.
func (r *Repository) LoadUnpacked(ctx context.Context, t restic.FileType, id restic.ID) ([]byte, error) {
	debug.Log("load %v with id %v", t, id)

	if t == restic.ConfigFile {
		id = restic.ID{}
	}

	buf, err := r.LoadRaw(ctx, t, id)
	if err != nil {
		return nil, err
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
	Has(backend.Handle) bool
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
		if cache.Has(backend.Handle{Type: restic.PackFile, Name: blob.PackID.String()}) {
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

	buf, err := r.loadBlob(ctx, blobs, buf)
	if err != nil {
		if r.Cache != nil {
			for _, blob := range blobs {
				h := backend.Handle{Type: restic.PackFile, Name: blob.PackID.String(), IsMetadata: blob.Type.IsMetadata()}
				// ignore errors as there's not much we can do here
				_ = r.Cache.Forget(h)
			}
		}

		buf, err = r.loadBlob(ctx, blobs, buf)
	}
	return buf, err
}

func (r *Repository) loadBlob(ctx context.Context, blobs []restic.PackedBlob, buf []byte) ([]byte, error) {
	var lastError error
	for _, blob := range blobs {
		debug.Log("blob %v found: %v", blob.BlobHandle, blob)
		// load blob from pack
		h := backend.Handle{Type: restic.PackFile, Name: blob.PackID.String(), IsMetadata: blob.Type.IsMetadata()}

		switch {
		case cap(buf) < int(blob.Length):
			buf = make([]byte, blob.Length)
		case len(buf) != int(blob.Length):
			buf = buf[:blob.Length]
		}

		_, err := backend.ReadAt(ctx, r.be, h, int64(blob.Offset), buf)
		if err != nil {
			debug.Log("error loading blob %v: %v", blob, err)
			lastError = err
			continue
		}

		it := newPackBlobIterator(blob.PackID, newByteReader(buf), uint(blob.Offset), []restic.Blob{blob.Blob}, r.key, r.getZstdDecoder())
		pbv, err := it.Next()

		if err == nil {
			err = pbv.Err
		}
		if err != nil {
			debug.Log("error decoding blob %v: %v", blob, err)
			lastError = err
			continue
		}

		plaintext := pbv.Plaintext
		if len(plaintext) > cap(buf) {
			return plaintext, nil
		}
		// move decrypted data to the start of the buffer
		buf = buf[:len(plaintext)]
		copy(buf, plaintext)
		return buf, nil
	}

	if lastError != nil {
		return nil, lastError
	}

	return nil, errors.Errorf("loading %v from %v packs failed", blobs[0].BlobHandle, len(blobs))
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
// is small enough, it will be packed together with other small blobs. The
// caller must ensure that the id matches the data. Returned is the size data
// occupies in the repo (compressed or not, including the encryption overhead).
func (r *Repository) saveAndEncrypt(ctx context.Context, t restic.BlobType, data []byte, id restic.ID) (size int, err error) {
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

	ciphertext := make([]byte, 0, crypto.CiphertextLength(len(data)))
	ciphertext = append(ciphertext, nonce...)

	// encrypt blob
	ciphertext = r.key.Seal(ciphertext, nonce, data, nil)

	if err := r.verifyCiphertext(ciphertext, uncompressedLength, id); err != nil {
		//nolint:revive // ignore linter warnings about error message spelling
		return 0, fmt.Errorf("Detected data corruption while saving blob %v: %w\nCorrupted blobs are either caused by hardware issues or software bugs. Please open an issue at https://github.com/restic/restic/issues/new/choose for further troubleshooting.", id, err)
	}

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

	return pm.SaveBlob(ctx, t, id, ciphertext, uncompressedLength)
}

func (r *Repository) verifyCiphertext(buf []byte, uncompressedLength int, id restic.ID) error {
	if r.opts.NoExtraVerify {
		return nil
	}

	nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
	plaintext, err := r.key.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}
	if uncompressedLength != 0 {
		// DecodeAll will allocate a slice if it is not large enough since it
		// knows the decompressed size (because we're using EncodeAll)
		plaintext, err = r.getZstdDecoder().DecodeAll(plaintext, nil)
		if err != nil {
			return fmt.Errorf("decompression failed: %w", err)
		}
	}
	if !restic.Hash(plaintext).Equal(id) {
		return errors.New("hash mismatch")
	}

	return nil
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
func (r *Repository) SaveUnpacked(ctx context.Context, t restic.FileType, buf []byte) (id restic.ID, err error) {
	p := buf
	if t != restic.ConfigFile {
		p, err = r.compressUnpacked(p)
		if err != nil {
			return restic.ID{}, err
		}
	}

	ciphertext := crypto.NewBlobBuffer(len(p))
	ciphertext = ciphertext[:0]
	nonce := crypto.NewRandomNonce()
	ciphertext = append(ciphertext, nonce...)

	ciphertext = r.key.Seal(ciphertext, nonce, p, nil)

	if err := r.verifyUnpacked(ciphertext, t, buf); err != nil {
		//nolint:revive // ignore linter warnings about error message spelling
		return restic.ID{}, fmt.Errorf("Detected data corruption while saving file of type %v: %w\nCorrupted data is either caused by hardware issues or software bugs. Please open an issue at https://github.com/restic/restic/issues/new/choose for further troubleshooting.", t, err)
	}

	if t == restic.ConfigFile {
		id = restic.ID{}
	} else {
		id = restic.Hash(ciphertext)
	}
	h := backend.Handle{Type: t, Name: id.String()}

	err = r.be.Save(ctx, h, backend.NewByteReader(ciphertext, r.be.Hasher()))
	if err != nil {
		debug.Log("error saving blob %v: %v", h, err)
		return restic.ID{}, err
	}

	debug.Log("blob %v saved", h)
	return id, nil
}

func (r *Repository) verifyUnpacked(buf []byte, t restic.FileType, expected []byte) error {
	if r.opts.NoExtraVerify {
		return nil
	}

	nonce, ciphertext := buf[:r.key.NonceSize()], buf[r.key.NonceSize():]
	plaintext, err := r.key.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}
	if t != restic.ConfigFile {
		plaintext, err = r.decompressUnpacked(plaintext)
		if err != nil {
			return fmt.Errorf("decompression failed: %w", err)
		}
	}

	if !bytes.Equal(plaintext, expected) {
		return errors.New("data mismatch")
	}
	return nil
}

func (r *Repository) RemoveUnpacked(ctx context.Context, t restic.FileType, id restic.ID) error {
	// TODO prevent everything except removing snapshots for non-repository code
	return r.be.Remove(ctx, backend.Handle{Type: t, Name: id.String()})
}

// Flush saves all remaining packs and the index
func (r *Repository) Flush(ctx context.Context) error {
	if err := r.flushPacks(ctx); err != nil {
		return err
	}

	return r.idx.SaveIndex(ctx, r)
}

func (r *Repository) StartPackUploader(ctx context.Context, wg *errgroup.Group) {
	if r.packerWg != nil {
		panic("uploader already started")
	}

	innerWg, ctx := errgroup.WithContext(ctx)
	r.packerWg = innerWg
	r.uploader = newPackerUploader(ctx, innerWg, r, r.be.Connections())
	r.treePM = newPackerManager(r.key, restic.TreeBlob, r.packSize(), r.uploader.QueuePacker)
	r.dataPM = newPackerManager(r.key, restic.DataBlob, r.packSize(), r.uploader.QueuePacker)

	wg.Go(func() error {
		return innerWg.Wait()
	})
}

// FlushPacks saves all remaining packs.
func (r *Repository) flushPacks(ctx context.Context) error {
	if r.packerWg == nil {
		return nil
	}

	err := r.treePM.Flush(ctx)
	if err != nil {
		return err
	}
	err = r.dataPM.Flush(ctx)
	if err != nil {
		return err
	}
	r.uploader.TriggerShutdown()
	err = r.packerWg.Wait()

	r.treePM = nil
	r.dataPM = nil
	r.uploader = nil
	r.packerWg = nil

	return err
}

func (r *Repository) Connections() uint {
	return r.be.Connections()
}

func (r *Repository) LookupBlob(tpe restic.BlobType, id restic.ID) []restic.PackedBlob {
	return r.idx.Lookup(restic.BlobHandle{Type: tpe, ID: id})
}

// LookupBlobSize returns the size of blob id.
func (r *Repository) LookupBlobSize(tpe restic.BlobType, id restic.ID) (uint, bool) {
	return r.idx.LookupSize(restic.BlobHandle{Type: tpe, ID: id})
}

// ListBlobs runs fn on all blobs known to the index. When the context is cancelled,
// the index iteration returns immediately with ctx.Err(). This blocks any modification of the index.
func (r *Repository) ListBlobs(ctx context.Context, fn func(restic.PackedBlob)) error {
	return r.idx.Each(ctx, fn)
}

func (r *Repository) ListPacksFromIndex(ctx context.Context, packs restic.IDSet) <-chan restic.PackBlobs {
	return r.idx.ListPacks(ctx, packs)
}

// SetIndex instructs the repository to use the given index.
func (r *Repository) SetIndex(i restic.MasterIndex) error {
	r.idx = i.(*index.MasterIndex)
	return r.prepareCache()
}

func (r *Repository) clearIndex() {
	r.idx = index.NewMasterIndex()
}

// LoadIndex loads all index files from the backend in parallel and stores them
func (r *Repository) LoadIndex(ctx context.Context, p *progress.Counter) error {
	debug.Log("Loading index")

	// reset in-memory index before loading it from the repository
	r.clearIndex()

	err := r.idx.Load(ctx, r, p, nil)
	if err != nil {
		return err
	}

	// Trigger GC to reset garbage collection threshold
	runtime.GC()

	if r.cfg.Version < 2 {
		// sanity check
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		invalidIndex := false
		err := r.idx.Each(ctx, func(blob restic.PackedBlob) {
			if blob.IsCompressed() {
				invalidIndex = true
			}
		})
		if err != nil {
			return err
		}
		if invalidIndex {
			return errors.New("index uses feature not supported by repository version 1")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// remove index files from the cache which have been removed in the repo
	return r.prepareCache()
}

// createIndexFromPacks creates a new index by reading all given pack files (with sizes).
// The index is added to the MasterIndex but not marked as finalized.
// Returned is the list of pack files which could not be read.
func (r *Repository) createIndexFromPacks(ctx context.Context, packsize map[restic.ID]int64, p *progress.Counter) (invalid restic.IDs, err error) {
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
				return ctx.Err()
			case ch <- FileInfo{id, size}:
			}
		}
		return nil
	})

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
			r.idx.StorePack(fi.ID, entries)
			p.Add(1)
		}

		return nil
	}

	// decoding the pack header is usually quite fast, thus we are primarily IO-bound
	workerCount := int(r.Connections())
	// run workers on ch
	for i := 0; i < workerCount; i++ {
		wg.Go(worker)
	}

	err = wg.Wait()
	if err != nil {
		return invalid, err
	}

	return invalid, nil
}

// prepareCache initializes the local cache. indexIDs is the list of IDs of
// index files still present in the repo.
func (r *Repository) prepareCache() error {
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

	oldKey := r.key
	oldKeyID := r.keyID

	r.key = key.master
	r.keyID = key.ID()
	cfg, err := restic.LoadConfig(ctx, r)
	if err != nil {
		r.key = oldKey
		r.keyID = oldKeyID

		if err == crypto.ErrUnauthenticated {
			return fmt.Errorf("config or key %v is damaged: %w", key.ID(), err)
		}
		return fmt.Errorf("config cannot be loaded: %w", err)
	}

	r.setConfig(cfg)
	return nil
}

// Init creates a new master key with the supplied password, initializes and
// saves the repository config.
func (r *Repository) Init(ctx context.Context, version uint, password string, chunkerPolynomial *chunker.Pol) error {
	if version > restic.MaxRepoVersion {
		return fmt.Errorf("repository version %v too high", version)
	}

	if version < restic.MinRepoVersion {
		return fmt.Errorf("repository version %v too low", version)
	}

	_, err := r.be.Stat(ctx, backend.Handle{Type: restic.ConfigFile})
	if err != nil && !r.be.IsNotExist(err) {
		return err
	}
	if err == nil {
		return errors.New("repository master key and config already initialized")
	}
	// double check to make sure that a repository is not accidentally reinitialized
	// if the backend somehow fails to stat the config file. An initialized repository
	// must always contain at least one key file.
	if err := r.List(ctx, restic.KeyFile, func(_ restic.ID, _ int64) error {
		return errors.New("repository already contains keys")
	}); err != nil {
		return err
	}
	// Also check for snapshots to detect repositories with a misconfigured retention
	// policy that deletes files older than x days. For such repositories usually the
	// config and key files are removed first and therefore the check would not detect
	// the old repository.
	if err := r.List(ctx, restic.SnapshotFile, func(_ restic.ID, _ int64) error {
		return errors.New("repository already contains snapshots")
	}); err != nil {
		return err
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
	r.keyID = key.ID()
	r.setConfig(cfg)
	return restic.SaveConfig(ctx, r, cfg)
}

// Key returns the current master key.
func (r *Repository) Key() *crypto.Key {
	return r.key
}

// KeyID returns the id of the current key in the backend.
func (r *Repository) KeyID() restic.ID {
	return r.keyID
}

// List runs fn for all files of type t in the repo.
func (r *Repository) List(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error {
	return r.be.List(ctx, t, func(fi backend.FileInfo) error {
		id, err := restic.ParseID(fi.Name)
		if err != nil {
			debug.Log("unable to parse %v as an ID", fi.Name)
			return nil
		}
		return fn(id, fi.Size)
	})
}

// ListPack returns the list of blobs saved in the pack id and the length of
// the pack header.
func (r *Repository) ListPack(ctx context.Context, id restic.ID, size int64) ([]restic.Blob, uint32, error) {
	h := backend.Handle{Type: restic.PackFile, Name: id.String()}

	entries, hdrSize, err := pack.List(r.Key(), backend.ReaderAt(ctx, r.be, h), size)
	if err != nil {
		if r.Cache != nil {
			// ignore error as there is not much we can do here
			_ = r.Cache.Forget(h)
		}

		// retry on error
		entries, hdrSize, err = pack.List(r.Key(), backend.ReaderAt(ctx, r.be, h), size)
	}
	return entries, hdrSize, err
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
// Also returns if the blob was already known before.
// If the blob was not known before, it returns the number of bytes the blob
// occupies in the repo (compressed or not, including encryption overhead).
func (r *Repository) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, size int, err error) {

	if int64(len(buf)) > math.MaxUint32 {
		return restic.ID{}, false, 0, fmt.Errorf("blob is larger than 4GB")
	}

	// compute plaintext hash if not already set
	if id.IsNull() {
		// Special case the hash calculation for all zero chunks. This is especially
		// useful for sparse files containing large all zero regions. For these we can
		// process chunks as fast as we can read the from disk.
		if len(buf) == chunker.MinSize && restic.ZeroPrefixLen(buf) == chunker.MinSize {
			newID = ZeroChunk()
		} else {
			newID = restic.Hash(buf)
		}
	} else {
		newID = id
	}

	// first try to add to pending blobs; if not successful, this blob is already known
	known = !r.idx.AddPending(restic.BlobHandle{ID: newID, Type: t})

	// only save when needed or explicitly told
	if !known || storeDuplicate {
		size, err = r.saveAndEncrypt(ctx, t, buf, newID)
	}

	return newID, known, size, err
}

type backendLoadFn func(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error
type loadBlobFn func(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error)

// Skip sections with more than 1MB unused blobs
const maxUnusedRange = 1 * 1024 * 1024

// LoadBlobsFromPack loads the listed blobs from the specified pack file. The plaintext blob is passed to
// the handleBlobFn callback or an error if decryption failed or the blob hash does not match.
// handleBlobFn is called at most once for each blob. If the callback returns an error,
// then LoadBlobsFromPack will abort and not retry it. The buf passed to the callback is only valid within
// this specific call. The callback must not keep a reference to buf.
func (r *Repository) LoadBlobsFromPack(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	return streamPack(ctx, r.be.Load, r.LoadBlob, r.getZstdDecoder(), r.key, packID, blobs, handleBlobFn)
}

func streamPack(ctx context.Context, beLoad backendLoadFn, loadBlobFn loadBlobFn, dec *zstd.Decoder, key *crypto.Key, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	if len(blobs) == 0 {
		// nothing to do
		return nil
	}

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})

	lowerIdx := 0
	lastPos := blobs[0].Offset
	const maxChunkSize = 2 * DefaultPackSize

	for i := 0; i < len(blobs); i++ {
		if blobs[i].Offset < lastPos {
			// don't wait for streamPackPart to fail
			return errors.Errorf("overlapping blobs in pack %v", packID)
		}

		chunkSizeAfter := (blobs[i].Offset + blobs[i].Length) - blobs[lowerIdx].Offset
		split := false
		// split if the chunk would become larger than maxChunkSize. Oversized chunks are
		// handled by the requirement that the chunk contains at least one blob (i > lowerIdx)
		if i > lowerIdx && chunkSizeAfter >= maxChunkSize {
			split = true
		}
		// skip too large gaps as a new request is typically much cheaper than data transfers
		if blobs[i].Offset-lastPos > maxUnusedRange {
			split = true
		}

		if split {
			// load everything up to the skipped file section
			err := streamPackPart(ctx, beLoad, loadBlobFn, dec, key, packID, blobs[lowerIdx:i], handleBlobFn)
			if err != nil {
				return err
			}
			lowerIdx = i
		}
		lastPos = blobs[i].Offset + blobs[i].Length
	}
	// load remainder
	return streamPackPart(ctx, beLoad, loadBlobFn, dec, key, packID, blobs[lowerIdx:], handleBlobFn)
}

func streamPackPart(ctx context.Context, beLoad backendLoadFn, loadBlobFn loadBlobFn, dec *zstd.Decoder, key *crypto.Key, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error {
	h := backend.Handle{Type: restic.PackFile, Name: packID.String(), IsMetadata: blobs[0].Type.IsMetadata()}

	dataStart := blobs[0].Offset
	dataEnd := blobs[len(blobs)-1].Offset + blobs[len(blobs)-1].Length

	debug.Log("streaming pack %v (%d to %d bytes), blobs: %v", packID, dataStart, dataEnd, len(blobs))

	data := make([]byte, int(dataEnd-dataStart))
	err := beLoad(ctx, h, int(dataEnd-dataStart), int64(dataStart), func(rd io.Reader) error {
		_, cerr := io.ReadFull(rd, data)
		return cerr
	})
	// prevent callbacks after cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		// the context is only still valid if handleBlobFn never returned an error
		if loadBlobFn != nil {
			// check whether we can get the remaining blobs somewhere else
			for _, entry := range blobs {
				buf, ierr := loadBlobFn(ctx, entry.Type, entry.ID, nil)
				err = handleBlobFn(entry.BlobHandle, buf, ierr)
				if err != nil {
					break
				}
			}
		}
		return errors.Wrap(err, "StreamPack")
	}

	it := newPackBlobIterator(packID, newByteReader(data), dataStart, blobs, key, dec)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		val, err := it.Next()
		if err == errPackEOF {
			break
		} else if err != nil {
			return err
		}

		if val.Err != nil && loadBlobFn != nil {
			var ierr error
			// check whether we can get a valid copy somewhere else
			buf, ierr := loadBlobFn(ctx, val.Handle.Type, val.Handle.ID, nil)
			if ierr == nil {
				// success
				val.Plaintext = buf
				val.Err = nil
			}
		}

		err = handleBlobFn(val.Handle, val.Plaintext, val.Err)
		if err != nil {
			return err
		}
		// ensure that each blob is only passed once to handleBlobFn
		blobs = blobs[1:]
	}

	return errors.Wrap(err, "StreamPack")
}

// discardReader allows the PackBlobIterator to perform zero copy
// reads if the underlying data source is a byte slice.
type discardReader interface {
	Discard(n int) (discarded int, err error)
	// ReadFull reads the next n bytes into a byte slice. The caller must not
	// retain a reference to the byte. Modifications are only allowed within
	// the boundaries of the returned slice.
	ReadFull(n int) (buf []byte, err error)
}

type byteReader struct {
	buf []byte
}

func newByteReader(buf []byte) *byteReader {
	return &byteReader{
		buf: buf,
	}
}

func (b *byteReader) Discard(n int) (discarded int, err error) {
	if len(b.buf) < n {
		return 0, io.ErrUnexpectedEOF
	}
	b.buf = b.buf[n:]
	return n, nil
}

func (b *byteReader) ReadFull(n int) (buf []byte, err error) {
	if len(b.buf) < n {
		return nil, io.ErrUnexpectedEOF
	}
	buf = b.buf[:n]
	b.buf = b.buf[n:]
	return buf, nil
}

type packBlobIterator struct {
	packID        restic.ID
	rd            discardReader
	currentOffset uint

	blobs []restic.Blob
	key   *crypto.Key
	dec   *zstd.Decoder

	decode []byte
}

type packBlobValue struct {
	Handle    restic.BlobHandle
	Plaintext []byte
	Err       error
}

var errPackEOF = errors.New("reached EOF of pack file")

func newPackBlobIterator(packID restic.ID, rd discardReader, currentOffset uint,
	blobs []restic.Blob, key *crypto.Key, dec *zstd.Decoder) *packBlobIterator {
	return &packBlobIterator{
		packID:        packID,
		rd:            rd,
		currentOffset: currentOffset,
		blobs:         blobs,
		key:           key,
		dec:           dec,
	}
}

// Next returns the next blob, an error or ErrPackEOF if all blobs were read
func (b *packBlobIterator) Next() (packBlobValue, error) {
	if len(b.blobs) == 0 {
		return packBlobValue{}, errPackEOF
	}

	entry := b.blobs[0]
	b.blobs = b.blobs[1:]

	skipBytes := int(entry.Offset - b.currentOffset)
	if skipBytes < 0 {
		return packBlobValue{}, fmt.Errorf("overlapping blobs in pack %v", b.packID)
	}

	_, err := b.rd.Discard(skipBytes)
	if err != nil {
		return packBlobValue{}, err
	}
	b.currentOffset = entry.Offset

	h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
	debug.Log("  process blob %v, skipped %d, %v", h, skipBytes, entry)

	buf, err := b.rd.ReadFull(int(entry.Length))
	if err != nil {
		debug.Log("    read error %v", err)
		return packBlobValue{}, fmt.Errorf("readFull: %w", err)
	}

	b.currentOffset = entry.Offset + entry.Length

	if int(entry.Length) <= b.key.NonceSize() {
		debug.Log("%v", b.blobs)
		return packBlobValue{}, fmt.Errorf("invalid blob length %v", entry)
	}

	// decryption errors are likely permanent, give the caller a chance to skip them
	nonce, ciphertext := buf[:b.key.NonceSize()], buf[b.key.NonceSize():]
	plaintext, err := b.key.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		err = fmt.Errorf("decrypting blob %v from %v failed: %w", h, b.packID.Str(), err)
	}
	if err == nil && entry.IsCompressed() {
		// DecodeAll will allocate a slice if it is not large enough since it
		// knows the decompressed size (because we're using EncodeAll)
		b.decode, err = b.dec.DecodeAll(plaintext, b.decode[:0])
		plaintext = b.decode
		if err != nil {
			err = fmt.Errorf("decompressing blob %v from %v failed: %w", h, b.packID.Str(), err)
		}
	}
	if err == nil {
		id := restic.Hash(plaintext)
		if !id.Equal(entry.ID) {
			debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
				h.Type, h.ID, b.packID.Str(), id)
			err = fmt.Errorf("read blob %v from %v: wrong data returned, hash is %v",
				h, b.packID.Str(), id)
		}
	}

	return packBlobValue{entry.BlobHandle, plaintext, err}, nil
}

var zeroChunkOnce sync.Once
var zeroChunkID restic.ID

// ZeroChunk computes and returns (cached) the ID of an all-zero chunk with size chunker.MinSize
func ZeroChunk() restic.ID {
	zeroChunkOnce.Do(func() {
		zeroChunkID = restic.Hash(make([]byte, chunker.MinSize))
	})
	return zeroChunkID
}
