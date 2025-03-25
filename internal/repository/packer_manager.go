package repository

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"
	"os"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/hashing"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository/pack"
)

// packer holds a pack.packer together with a hash writer.
type packer struct {
	*pack.Packer
	tmpfile *os.File
	bufWr   *bufio.Writer
}

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	tpe     restic.BlobType
	key     *crypto.Key
	queueFn func(ctx context.Context, t restic.BlobType, p *packer) error

	pm       sync.Mutex
	packers  []*packer
	packSize uint
}

const defaultPackerCount = 2

// newPackerManager returns a new packer manager which writes temporary files
// to a temporary directory
func newPackerManager(key *crypto.Key, tpe restic.BlobType, packSize uint, packerCount int, queueFn func(ctx context.Context, t restic.BlobType, p *packer) error) *packerManager {
	return &packerManager{
		tpe:      tpe,
		key:      key,
		queueFn:  queueFn,
		packers:  make([]*packer, packerCount),
		packSize: packSize,
	}
}

func (r *packerManager) Flush(ctx context.Context) error {
	r.pm.Lock()
	defer r.pm.Unlock()

	pendingPackers, err := r.mergePackers()
	if err != nil {
		return err
	}

	for _, packer := range pendingPackers {
		debug.Log("manually flushing pending pack")
		err := r.queueFn(ctx, r.tpe, packer)
		if err != nil {
			return err
		}
	}
	return nil
}

// mergePackers merges small pack files before those are uploaded by Flush(). The main
// purpose of this method is to reduce information leaks if a small file is backed up
// and the blobs end up in spearate pack files. If the file only consists of two blobs
// this would leak the size of the individual blobs.
func (r *packerManager) mergePackers() ([]*packer, error) {
	pendingPackers := []*packer{}
	var p *packer
	for i, packer := range r.packers {
		if packer == nil {
			continue
		}

		r.packers[i] = nil
		if p == nil {
			p = packer
		} else if p.Size()+packer.Size() < r.packSize {
			// merge if the result stays below the target pack size
			err := packer.bufWr.Flush()
			if err != nil {
				return nil, err
			}
			_, err = packer.tmpfile.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}

			err = p.Merge(packer.Packer, packer.tmpfile)
			if err != nil {
				return nil, err
			}
		} else {
			pendingPackers = append(pendingPackers, p)
			p = packer
		}
	}
	if p != nil {
		pendingPackers = append(pendingPackers, p)
	}
	return pendingPackers, nil
}

func (r *packerManager) SaveBlob(ctx context.Context, t restic.BlobType, id restic.ID, ciphertext []byte, uncompressedLength int) (int, error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	packer, err := r.pickPacker(len(ciphertext))
	if err != nil {
		return 0, err
	}

	// save ciphertext
	// Add only appends bytes in memory to avoid being a scaling bottleneck
	size, err := packer.Add(t, id, ciphertext, uncompressedLength)
	if err != nil {
		return 0, err
	}

	// if the pack and header is not full enough, put back to the list
	if packer.Size() < r.packSize && !packer.HeaderFull() {
		debug.Log("pack is not full enough (%d bytes)", packer.Size())
		return size, nil
	}

	// forget full packer
	r.forgetPacker(packer)

	// call while holding lock to prevent findPacker from creating new packers if the uploaders are busy
	// else write the pack to the backend
	err = r.queueFn(ctx, t, packer)
	if err != nil {
		return 0, err
	}

	return size + packer.HeaderOverhead(), nil
}

func randomInt(max int) (int, error) {
	rangeSize := big.NewInt(int64(max))
	randomInt, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return 0, err
	}
	return int(randomInt.Int64()), nil
}

// pickPacker returns or creates a randomly selected packer into which the blob should be stored. If the
// ciphertext is larger than the packSize, a new packer is returned.
func (r *packerManager) pickPacker(ciphertextLen int) (*packer, error) {
	// use separate packer if compressed length is larger than the packsize
	// this speeds up the garbage collection of oversized blobs and reduces the cache size
	// as the oversize blobs are only downloaded if necessary
	if ciphertextLen >= int(r.packSize) {
		return r.newPacker()
	}

	// randomly distribute blobs onto multiple packer instances. This makes it harder for
	// an attacker to learn at which points a file was chunked and therefore mitigates the attack described in
	// https://www.daemonology.net/blog/chunking-attacks.pdf .
	// See https://github.com/restic/restic/issues/5291#issuecomment-2746146193 for details on the mitigation.
	idx, err := randomInt(len(r.packers))
	if err != nil {
		return nil, err
	}

	// retrieve packer or get a new one
	packer := r.packers[idx]
	if packer == nil {
		packer, err = r.newPacker()
		if err != nil {
			return nil, err
		}
		r.packers[idx] = packer
	}
	return packer, nil
}

// forgetPacker drops the given packer from the internal list. This is used to forget full packers.
func (r *packerManager) forgetPacker(packer *packer) {
	for i, p := range r.packers {
		if packer == p {
			r.packers[i] = nil
		}
	}
}

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) newPacker() (pck *packer, err error) {
	debug.Log("create new pack")
	tmpfile, err := fs.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	bufWr := bufio.NewWriter(tmpfile)
	p := pack.NewPacker(r.key, bufWr)
	pck = &packer{
		Packer:  p,
		tmpfile: tmpfile,
		bufWr:   bufWr,
	}

	return pck, nil
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(ctx context.Context, t restic.BlobType, p *packer) error {
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
	rd, err = backend.NewFileReader(p.tmpfile, nil)
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
	h := backend.Handle{Type: backend.PackFile, Name: id.String(), IsMetadata: t.IsMetadata()}
	var beHash []byte
	if beHr != nil {
		beHash = beHr.Sum(nil)
	}
	rrd, err := backend.NewFileReader(p.tmpfile, beHash)
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

	// update blobs in the index
	debug.Log("  updating blobs %v to pack %v", p.Packer.Blobs(), id)
	return r.idx.StorePack(ctx, id, p.Packer.Blobs(), &internalRepository{r})
}
