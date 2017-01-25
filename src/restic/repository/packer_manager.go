package repository

import (
	"crypto/sha256"
	"io"
	"io/ioutil"
	"os"
	"restic"
	"sync"

	"restic/errors"
	"restic/hashing"

	"restic/crypto"
	"restic/debug"
	"restic/fs"
	"restic/pack"
)

// Saver implements saving data in a backend.
type Saver interface {
	Save(restic.Handle, io.Reader) error
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

	pool sync.Pool
}

const minPackSize = 4 * 1024 * 1024
const maxPackSize = 16 * 1024 * 1024
const maxPackers = 200

// newPackerManager returns an new packer manager which writes temporary files
// to a temporary directory
func newPackerManager(be Saver, key *crypto.Key) *packerManager {
	return &packerManager{
		be:  be,
		key: key,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, (minPackSize+maxPackSize)/2)
			},
		},
	}
}

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) findPacker(size uint) (packer *Packer, err error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packers) > 0 {
		debug.Log("searching packer for %d bytes\n", size)
		for i, p := range r.packers {
			if p.Packer.Size()+size < maxPackSize {
				debug.Log("found packer %v", p)
				// remove from list
				r.packers = append(r.packers[:i], r.packers[i+1:]...)
				return p, nil
			}
		}
	}

	// no suitable packer found, return new
	debug.Log("create new pack for %d bytes", size)
	tmpfile, err := ioutil.TempFile("", "restic-temp-pack-")
	if err != nil {
		return nil, errors.Wrap(err, "ioutil.TempFile")
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
func (r *Repository) savePacker(p *Packer) error {
	debug.Log("save packer with %d blobs\n", p.Packer.Count())
	_, err := p.Packer.Finalize()
	if err != nil {
		return err
	}

	_, err = p.tmpfile.Seek(0, 0)
	if err != nil {
		return errors.Wrap(err, "Seek")
	}

	id := restic.IDFromHash(p.hw.Sum(nil))
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	err = r.be.Save(h, p.tmpfile)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return err
	}

	debug.Log("saved as %v", h)

	err = p.tmpfile.Close()
	if err != nil {
		return errors.Wrap(err, "close tempfile")
	}

	err = fs.Remove(p.tmpfile.Name())
	if err != nil {
		return errors.Wrap(err, "Remove")
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

	return nil
}

// countPacker returns the number of open (unfinished) packers.
func (r *packerManager) countPacker() int {
	r.pm.Lock()
	defer r.pm.Unlock()

	return len(r.packers)
}
