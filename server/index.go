package server

import (
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// Index holds a lookup table for id -> pack.
type Index struct {
	m    sync.Mutex
	pack map[string]indexEntry
}

type indexEntry struct {
	tpe    pack.BlobType
	packID backend.ID
	offset uint
	length uint
	old    bool
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		pack: make(map[string]indexEntry),
	}
}

func (idx *Index) store(t pack.BlobType, id, pack backend.ID, offset, length uint, old bool) {
	idx.pack[id.String()] = indexEntry{
		tpe:    t,
		packID: pack,
		offset: offset,
		length: length,
		old:    old,
	}
}

// Store remembers the id and pack in the index.
func (idx *Index) Store(t pack.BlobType, id, pack backend.ID, offset, length uint) {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("Index.Store", "pack %v contains id %v, offset %v, length %v",
		pack.Str(), id.Str(), offset, length)

	idx.store(t, id, pack, offset, length, false)
}

// Remove removes the pack ID from the index.
func (idx *Index) Remove(packID backend.ID) {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("Index.Remove", "id %v removed", packID.Str())

	s := packID.String()
	if _, ok := idx.pack[s]; ok {
		delete(idx.pack, s)
	}
}

// Lookup returns the pack for the id.
func (idx *Index) Lookup(id backend.ID) (packID backend.ID, tpe pack.BlobType, offset, length uint, err error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if p, ok := idx.pack[id.String()]; ok {
		debug.Log("Index.Lookup", "id %v found in pack %v at %d, length %d",
			id.Str(), p.packID.Str(), p.offset, p.length)
		return p.packID, p.tpe, p.offset, p.length, nil
	}

	debug.Log("Index.Lookup", "id %v not found", id.Str())
	return nil, pack.Data, 0, 0, errors.New("id not found")
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id backend.ID) bool {
	_, _, _, _, err := idx.Lookup(id)
	if err == nil {
		return true
	}

	return false
}

// Each returns a channel that yields all blobs known to the index. If done is
// closed, the background goroutine terminates. This blocks any modification of
// the index.
func (idx *Index) Each(done chan struct{}) <-chan pack.Blob {
	idx.m.Lock()

	ch := make(chan pack.Blob)

	go func() {
		defer idx.m.Unlock()
		defer func() {
			close(ch)
		}()

		for ids, blob := range idx.pack {
			id, err := backend.ParseID(ids)
			if err != nil {
				// ignore invalid IDs
				continue
			}

			select {
			case <-done:
				return
			case ch <- pack.Blob{
				ID:     id,
				Offset: blob.offset,
				Type:   blob.tpe,
				Length: uint32(blob.length),
			}:
			}
		}
	}()

	return ch
}

type packJSON struct {
	ID    string     `json:"id"`
	Blobs []blobJSON `json:"blobs"`
}

type blobJSON struct {
	ID     string        `json:"id"`
	Type   pack.BlobType `json:"type"`
	Offset uint          `json:"offset"`
	Length uint          `json:"length"`
}

// Encode writes the JSON serialization of the index to the writer w. This
// serialization only contains new blobs added via idx.Store(), not old ones
// introduced via DecodeIndex().
func (idx *Index) Encode(w io.Writer) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	list := []*packJSON{}
	packs := make(map[string]*packJSON)

	for id, blob := range idx.pack {
		if blob.old {
			continue
		}

		// see if pack is already in map
		p, ok := packs[blob.packID.String()]
		if !ok {
			// else create new pack
			p = &packJSON{ID: blob.packID.String()}

			// and append it to the list and map
			list = append(list, p)
			packs[p.ID] = p
		}

		// add blob
		p.Blobs = append(p.Blobs, blobJSON{
			ID:     id,
			Type:   blob.tpe,
			Offset: blob.offset,
			Length: blob.length,
		})
	}

	enc := json.NewEncoder(w)
	return enc.Encode(list)
}

// DecodeIndex loads and unserializes an index from rd.
func DecodeIndex(rd io.Reader) (*Index, error) {
	list := []*packJSON{}

	dec := json.NewDecoder(rd)
	err := dec.Decode(&list)
	if err != nil {
		return nil, err
	}

	idx := NewIndex()
	for _, pack := range list {
		packID, err := backend.ParseID(pack.ID)
		if err != nil {
			return nil, err
		}

		for _, blob := range pack.Blobs {
			blobID, err := backend.ParseID(blob.ID)
			if err != nil {
				return nil, err
			}

			idx.store(blob.Type, blobID, packID, blob.Offset, blob.Length, true)
		}
	}

	return idx, err
}
