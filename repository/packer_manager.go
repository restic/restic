package repository

import (
	"sync"

	"github.com/restic/chunker"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	be    backend.Backend
	key   *crypto.Key
	pm    sync.Mutex
	packs []*pack.Packer
}

const minPackSize = 4 * chunker.MiB
const maxPackSize = 16 * chunker.MiB
const maxPackers = 200

// findPacker returns a packer for a new blob of size bytes. Either a new one is
// created or one is returned that already has some blobs.
func (r *packerManager) findPacker(size uint) (*pack.Packer, error) {
	r.pm.Lock()
	defer r.pm.Unlock()

	// search for a suitable packer
	if len(r.packs) > 0 {
		debug.Log("Repo.findPacker", "searching packer for %d bytes\n", size)
		for i, p := range r.packs {
			if p.Size()+size < maxPackSize {
				debug.Log("Repo.findPacker", "found packer %v", p)
				// remove from list
				r.packs = append(r.packs[:i], r.packs[i+1:]...)
				return p, nil
			}
		}
	}

	// no suitable packer found, return new
	blob, err := r.be.Create()
	if err != nil {
		return nil, err
	}
	debug.Log("Repo.findPacker", "create new pack %p for %d bytes", blob, size)
	return pack.NewPacker(r.key, blob), nil
}

// insertPacker appends p to s.packs.
func (r *packerManager) insertPacker(p *pack.Packer) {
	r.pm.Lock()
	defer r.pm.Unlock()

	r.packs = append(r.packs, p)
	debug.Log("Repo.insertPacker", "%d packers\n", len(r.packs))
}

// savePacker stores p in the backend.
func (r *Repository) savePacker(p *pack.Packer) error {
	debug.Log("Repo.savePacker", "save packer with %d blobs\n", p.Count())
	_, err := p.Finalize()
	if err != nil {
		return err
	}

	// move file to the final location
	sid := p.ID()
	err = p.Writer().(backend.Blob).Finalize(backend.Data, sid.String())
	if err != nil {
		debug.Log("Repo.savePacker", "blob Finalize() error: %v", err)
		return err
	}

	debug.Log("Repo.savePacker", "saved as %v", sid.Str())

	// update blobs in the index
	for _, b := range p.Blobs() {
		debug.Log("Repo.savePacker", "  updating blob %v to pack %v", b.ID.Str(), sid.Str())
		r.idx.Current().Store(PackedBlob{
			Type:   b.Type,
			ID:     b.ID,
			PackID: sid,
			Offset: b.Offset,
			Length: uint(b.Length),
		})
		r.idx.RemoveFromInFlight(b.ID)
	}

	return nil
}

// countPacker returns the number of open (unfinished) packers.
func (r *packerManager) countPacker() int {
	r.pm.Lock()
	defer r.pm.Unlock()

	return len(r.packs)
}
