package repository

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
	"sync"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/hashing"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"
)

// Saver implements saving data in a backend.
type Saver interface {
	Save(context.Context, restic.Handle, io.Reader) error
}

// Packer holds a pack.Packer together with a hash writer.
type Packer struct {
	*pack.Packer
	hw      *hashing.Writer
	tmpfile *os.File
}

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	be      Saver
	key     *crypto.Key
	pm      sync.Mutex
	packers []*Packer
}

const minPackSize = 4 * 1024 * 1024

// newPackerManager returns an new packer manager which writes temporary files
// to a temporary directory
func newPackerManager(be Saver, key *crypto.Key) *packerManager {
	return &packerManager{
		be:  be,
		key: key,
	}
}

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) findPacker() (packer *Packer, err error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packers) > 0 {
		p := r.packers[0]
		r.packers = r.packers[1:]
		return p, nil
	}

	// no suitable packer found, return new
	debug.Log("create new pack")
	tmpfile, err := fs.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.Wrap(err, "fs.TempFile")
	}

	hw := hashing.NewWriter(tmpfile, sha256.New())
	p := pack.NewPacker(r.key, hw)
	packer = &Packer{
		Packer:  p,
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
func (r *Repository) savePacker(t restic.BlobType, p *Packer) (uint64, error) {
	debug.Log("save packer for %v with %d blobs (%d bytes)\n", t, p.Packer.Count(), p.Packer.Size())
	bytesWritten, err := p.Packer.Finalize()
	if err != nil {
		return bytesWritten, err
	}

	_, err = p.tmpfile.Seek(0, 0)
	if err != nil {
		return bytesWritten, errors.Wrap(err, "Seek")
	}

	id := restic.IDFromHash(p.hw.Sum(nil))
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	err = r.be.Save(context.TODO(), h, p.tmpfile)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return bytesWritten, err
	}

	debug.Log("saved as %v", h)

	if t == restic.TreeBlob && r.Cache != nil {
		debug.Log("saving tree pack file in cache")

		_, err = p.tmpfile.Seek(0, 0)
		if err != nil {
			return bytesWritten, errors.Wrap(err, "Seek")
		}

		err := r.Cache.Save(h, p.tmpfile)
		if err != nil {
			return bytesWritten, err
		}
	}

	err = p.tmpfile.Close()
	if err != nil {
		return bytesWritten, errors.Wrap(err, "close tempfile")
	}

	err = fs.RemoveIfExists(p.tmpfile.Name())
	if err != nil {
		return bytesWritten, errors.Wrap(err, "Remove")
	}

	// update blobs in the index
	for _, b := range p.Packer.Blobs() {
		debug.Log("  updating blob %v to pack %v", b.ID.Str(), id.Str())
		r.idx.Store(restic.PackedBlob{
			Blob: restic.Blob{
				Type:   b.Type,
				ID:     b.ID,
				Offset: b.Offset,
				Length: uint(b.Length),
			},
			PackID: id,
		})
	}

	return bytesWritten, nil
}

// countPacker returns the number of open (unfinished) packers.
func (r *packerManager) countPacker() int {
	r.pm.Lock()
	defer r.pm.Unlock()

	return len(r.packers)
}
