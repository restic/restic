package repository

import (
	"context"
	"hash"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/hashing"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"

	"github.com/minio/sha256-simd"
)

// Packer holds a pack.Packer together with a hash writer.
type Packer struct {
	*pack.Packer
	hw      *hashing.Writer
	beHw    *hashing.Writer
	tmpfile *os.File
}

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	tpe      restic.BlobType
	key      *crypto.Key
	hasherFn func() hash.Hash
	queueFn  func(ctx context.Context, t restic.BlobType, p *Packer) error

	pm      sync.Mutex
	packers []*Packer
}

const minPackSize = 4 * 1024 * 1024

// newPackerManager returns an new packer manager which writes temporary files
// to a temporary directory
func newPackerManager(key *crypto.Key, hasherFn func() hash.Hash, tpe restic.BlobType, queueFn func(ctx context.Context, t restic.BlobType, p *Packer) error) *packerManager {
	return &packerManager{
		tpe:      tpe,
		key:      key,
		hasherFn: hasherFn,
		queueFn:  queueFn,
	}
}

func (r *packerManager) Flush(ctx context.Context) error {
	r.pm.Lock()
	defer r.pm.Unlock()

	debug.Log("manually flushing %d packs", len(r.packers))
	for _, packer := range r.packers {
		err := r.queueFn(ctx, r.tpe, packer)
		if err != nil {
			return err
		}
	}
	r.packers = r.packers[:0]
	return nil
}

func (r *packerManager) SaveBlob(ctx context.Context, t restic.BlobType, id restic.ID, ciphertext []byte, uncompressedLength int) (int, error) {
	packer, err := r.findPacker()
	if err != nil {
		return 0, err
	}

	// save ciphertext
	size, err := packer.Add(t, id, ciphertext, uncompressedLength)
	if err != nil {
		return 0, err
	}

	// if the pack is not full enough, put back to the list
	if packer.Size() < minPackSize {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		r.insertPacker(packer)
		return size, nil
	}

	// else write the pack to the backend
	err = r.queueFn(ctx, t, packer)
	if err != nil {
		return 0, err
	}

	return size + packer.HeaderOverhead(), nil
}

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) findPacker() (packer *Packer, err error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packers) > 0 {
		p := r.packers[0]
		last := len(r.packers) - 1
		r.packers[0] = r.packers[last]
		r.packers[last] = nil // Allow GC of stale reference.
		r.packers = r.packers[:last]
		return p, nil
	}

	// no suitable packer found, return new
	debug.Log("create new pack")
	tmpfile, err := fs.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.Wrap(err, "fs.TempFile")
	}

	w := io.Writer(tmpfile)
	beHasher := r.hasherFn()
	var beHw *hashing.Writer
	if beHasher != nil {
		beHw = hashing.NewWriter(w, beHasher)
		w = beHw
	}

	hw := hashing.NewWriter(w, sha256.New())
	p := pack.NewPacker(r.key, hw)
	packer = &Packer{
		Packer:  p,
		beHw:    beHw,
		hw:      hw,
		tmpfile: tmpfile,
	}

	return packer, nil
}

// insertPacker appends p to s.packs.
func (r *packerManager) insertPacker(p *Packer) {
	r.pm.Lock()
	defer r.pm.Unlock()

	r.packers = append(r.packers, p)
	debug.Log("%d packers\n", len(r.packers))
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(ctx context.Context, t restic.BlobType, p *Packer) error {
	debug.Log("save packer for %v with %d blobs (%d bytes)\n", t, p.Packer.Count(), p.Packer.Size())
	err := p.Packer.Finalize()
	if err != nil {
		return err
	}

	id := restic.IDFromHash(p.hw.Sum(nil))
	h := restic.Handle{Type: restic.PackFile, Name: id.String(),
		ContainedBlobType: t}
	var beHash []byte
	if p.beHw != nil {
		beHash = p.beHw.Sum(nil)
	}
	rd, err := restic.NewFileReader(p.tmpfile, beHash)
	if err != nil {
		return err
	}

	err = r.be.Save(ctx, h, rd)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return err
	}

	debug.Log("saved as %v", h)

	err = p.tmpfile.Close()
	if err != nil {
		return errors.Wrap(err, "close tempfile")
	}

	// on windows the tempfile is automatically deleted on close
	if runtime.GOOS != "windows" {
		err = fs.RemoveIfExists(p.tmpfile.Name())
		if err != nil {
			return errors.Wrap(err, "Remove")
		}
	}

	// update blobs in the index
	debug.Log("  updating blobs %v to pack %v", p.Packer.Blobs(), id)
	r.idx.StorePack(id, p.Packer.Blobs())

	// Save index if full
	if r.noAutoIndexUpdate {
		return nil
	}
	return r.idx.SaveFullIndex(ctx, r)
}

// countPacker returns the number of open (unfinished) packers.
func (r *packerManager) countPacker() int {
	r.pm.Lock()
	defer r.pm.Unlock()

	return len(r.packers)
}
