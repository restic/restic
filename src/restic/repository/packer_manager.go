package repository

import (
	"sync"

	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
)

// packerManager keeps a list of open packs and creates new on demand.
type packerManager struct {
	be    backend.Backend
	key   *crypto.Key
	pm    sync.Mutex
	packs []*pack.Packer
}

const minPackSize = 4 * 1024 * 1024
const maxPackSize = 16 * 1024 * 1024
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
	debug.Log("Repo.findPacker", "create new pack for %d bytes", size)
	return pack.NewPacker(r.key, nil), nil
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
	data, err := p.Finalize()
	if err != nil {
		return err
	}

	id := backend.Hash(data)
	h := backend.Handle{Type: backend.Data, Name: id.String()}

	err = r.be.Save(h, data)
	if err != nil {
		debug.Log("Repo.savePacker", "Save(%v) error: %v", h, err)
		return err
	}

	debug.Log("Repo.savePacker", "saved as %v", h)

	// update blobs in the index
	for _, b := range p.Blobs() {
		debug.Log("Repo.savePacker", "  updating blob %v to pack %v", b.ID.Str(), id.Str())
		r.idx.Current().Store(PackedBlob{
			Type:   b.Type,
			ID:     b.ID,
			PackID: id,
			Offset: b.Offset,
			Length: uint(b.Length),
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
