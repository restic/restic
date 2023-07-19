package repository

import (
	"bufio"
	"context"
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
	tmpfile *os.File
	bufWr   *bufio.Writer
}

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	tpe     restic.BlobType
	key     *crypto.Key
	queueFn func(ctx context.Context, t restic.BlobType, p *Packer) error

	pm       sync.Mutex
	packer   *Packer
	packSize int
}

// newPackerManager returns an new packer manager which writes temporary files
// to a temporary directory
func newPackerManager(key *crypto.Key, tpe restic.BlobType, packSize uint, queueFn func(ctx context.Context, t restic.BlobType, p *Packer) error) *packerManager {
	return &packerManager{
		tpe:      tpe,
		key:      key,
		queueFn:  queueFn,
		packSize: int(packSize),
	}
}

func (r *packerManager) Flush(ctx context.Context) error {
	r.pm.Lock()
	defer r.pm.Unlock()

	if r.packer != nil {
		debug.Log("manually flushing pending pack")
		err := r.queueFn(ctx, r.tpe, r.packer)
		if err != nil {
			return err
		}
		r.packer = nil
	}
	return nil
}

func (r *packerManager) SaveBlob(ctx context.Context, t restic.BlobType, id restic.ID, ciphertext []byte, uncompressedLength int) (int, error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	var err error
	packer := r.packer
	flush := false

	if packer != nil {
		if packer.HeaderFull() {
			debug.Log("pending pack (%d bytes) header is full", packer.Size())
			flush = true
		} else if packer.Size()+len(ciphertext) > r.packSize {
			debug.Log("pending pack (%d bytes) is going to be oversized with the blob (%d bytes)", packer.Size(), len(ciphertext))
			flush = true
		}
	}

	if flush {
		debug.Log("auto flushing pending pack")
		// write the pack to the backend
		err = r.queueFn(ctx, r.tpe, packer)
		if err != nil {
			return 0, err
		}
		// forget full packer
		r.packer = nil
	}

	if r.packer == nil {
		packer, err = r.newPacker()
		if err != nil {
			return 0, err
		}
		// remember packer
		r.packer = packer
	}

	// save ciphertext
	// Add only appends bytes in memory to avoid being a scaling bottleneck
	size, err := packer.Add(t, id, ciphertext, uncompressedLength)
	if err != nil {
		return 0, err
	}

	if flush {
		// we count here header size of the previous pack actually
		// but just for statistics (how it is used externally) it's ok
		size += packer.HeaderOverhead()
	}
	return size, nil
}

// newPacker creates and returns a new packer for blobs
func (r *packerManager) newPacker() (packer *Packer, err error) {
	debug.Log("create new pack")
	tmpfile, err := fs.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	bufWr := bufio.NewWriter(tmpfile)
	p := pack.NewPacker(r.key, bufWr)
	packer = &Packer{
		Packer:  p,
		tmpfile: tmpfile,
		bufWr:   bufWr,
	}

	return packer, nil
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(ctx context.Context, t restic.BlobType, p *Packer) error {
	debug.Log("save packer for %v with %d blobs (%d bytes)\n", t, p.Packer.Count(), p.Packer.Size())
	err := p.Packer.Finalize()
	if err != nil {
		return err
	}
	err = p.bufWr.Flush()
	if err != nil {
		return err
	}

	// calculate sha256 hash in a second pass
	var rd io.Reader
	rd, err = restic.NewFileReader(p.tmpfile, nil)
	if err != nil {
		return err
	}
	beHasher := r.be.Hasher()
	var beHr *hashing.Reader
	if beHasher != nil {
		beHr = hashing.NewReader(rd, beHasher)
		rd = beHr
	}

	hr := hashing.NewReader(rd, sha256.New())
	_, err = io.Copy(io.Discard, hr)
	if err != nil {
		return err
	}

	id := restic.IDFromHash(hr.Sum(nil))
	h := restic.Handle{Type: restic.PackFile, Name: id.String(), ContainedBlobType: t}
	var beHash []byte
	if beHr != nil {
		beHash = beHr.Sum(nil)
	}
	rrd, err := restic.NewFileReader(p.tmpfile, beHash)
	if err != nil {
		return err
	}

	err = r.be.Save(ctx, h, rrd)
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
			return errors.WithStack(err)
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
