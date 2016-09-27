package repository

import (
	"io"
	"io/ioutil"
	"os"
	"restic"
	"sync"

	"restic/errors"

	"restic/crypto"
	"restic/debug"
	"restic/fs"
	"restic/pack"
)

// Saver implements saving data in a backend.
type Saver interface {
	Save(h restic.Handle, jp []byte) error
}

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	be    Saver
	key   *crypto.Key
	pm    sync.Mutex
	packs []*pack.Packer

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
func (r *packerManager) findPacker(size uint) (packer *pack.Packer, err error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packs) > 0 {
		debug.Log("searching packer for %d bytes\n", size)
		for i, p := range r.packs {
			if p.Size()+size < maxPackSize {
				debug.Log("found packer %v", p)
				// remove from list
				r.packs = append(r.packs[:i], r.packs[i+1:]...)
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

	return pack.NewPacker(r.key, tmpfile), nil
}

// insertPacker appends p to s.packs.
func (r *packerManager) insertPacker(p *pack.Packer) {
	r.pm.Lock()
	defer r.pm.Unlock()

	r.packs = append(r.packs, p)
	debug.Log("%d packers\n", len(r.packs))
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(p *pack.Packer) error {
	debug.Log("save packer with %d blobs\n", p.Count())
	n, err := p.Finalize()
	if err != nil {
		return err
	}

	tmpfile := p.Writer().(*os.File)
	f, err := fs.Open(tmpfile.Name())
	if err != nil {
		return errors.Wrap(err, "Open")
	}

	data := make([]byte, n)
	m, err := io.ReadFull(f, data)
	if err != nil {
		return errors.Wrap(err, "ReadFul")
	}

	if uint(m) != n {
		return errors.Errorf("read wrong number of bytes from %v: want %v, got %v", tmpfile.Name(), n, m)
	}

	if err = f.Close(); err != nil {
		return errors.Wrap(err, "Close")
	}

	id := restic.Hash(data)
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}

	err = r.be.Save(h, data)
	if err != nil {
		debug.Log("Save(%v) error: %v", h, err)
		return err
	}

	debug.Log("saved as %v", h)

	err = fs.Remove(tmpfile.Name())
	if err != nil {
		return errors.Wrap(err, "Remove")
	}

	// update blobs in the index
	for _, b := range p.Blobs() {
		debug.Log("  updating blob %v to pack %v", b.ID.Str(), id.Str())
		r.idx.Current().Store(restic.PackedBlob{
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

	return len(r.packs)
}
