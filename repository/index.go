package repository

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// Index holds a lookup table for id -> pack.
type Index struct {
	m    sync.Mutex
	pack map[backend.ID]indexEntry
}

type indexEntry struct {
	tpe    pack.BlobType
	packID *backend.ID
	offset uint
	length uint
	old    bool
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		pack: make(map[backend.ID]indexEntry),
	}
}

func (idx *Index) store(t pack.BlobType, id backend.ID, pack *backend.ID, offset, length uint, old bool) {
	idx.pack[id] = indexEntry{
		tpe:    t,
		packID: pack,
		offset: offset,
		length: length,
		old:    old,
	}
}

// Store remembers the id and pack in the index.
func (idx *Index) Store(t pack.BlobType, id backend.ID, pack *backend.ID, offset, length uint) {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("Index.Store", "pack %v contains id %v (%v), offset %v, length %v",
		pack.Str(), id.Str(), t, offset, length)

	idx.store(t, id, pack, offset, length, false)
}

// Remove removes the pack ID from the index.
func (idx *Index) Remove(packID backend.ID) {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("Index.Remove", "id %v removed", packID.Str())

	if _, ok := idx.pack[packID]; ok {
		delete(idx.pack, packID)
	}
}

// Lookup returns the pack for the id.
func (idx *Index) Lookup(id backend.ID) (packID *backend.ID, tpe pack.BlobType, offset, length uint, err error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if p, ok := idx.pack[id]; ok {
		debug.Log("Index.Lookup", "id %v found in pack %v at %d, length %d",
			id.Str(), p.packID.Str(), p.offset, p.length)
		return p.packID, p.tpe, p.offset, p.length, nil
	}

	debug.Log("Index.Lookup", "id %v not found", id.Str())
	return nil, pack.Data, 0, 0, fmt.Errorf("id %v not found in index", id)
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id backend.ID) bool {
	_, _, _, _, err := idx.Lookup(id)
	if err == nil {
		return true
	}

	return false
}

// LookupSize returns the length of the cleartext content behind the
// given id
func (idx *Index) LookupSize(id backend.ID) (cleartextLength uint, err error) {
	_, _, _, encryptedLength, err := idx.Lookup(id)
	if err != nil {
		return 0, err
	}
	return encryptedLength - crypto.Extension, nil
}

// Merge loads all items from other into idx.
func (idx *Index) Merge(other *Index) {
	debug.Log("Index.Merge", "Merge index with %p", other)
	idx.m.Lock()
	defer idx.m.Unlock()

	for k, v := range other.pack {
		if _, ok := idx.pack[k]; ok {
			debug.Log("Index.Merge", "index already has key %v, updating", k[:8])
		}

		idx.pack[k] = v
	}
	debug.Log("Index.Merge", "done merging index")
}

// PackedBlob is a blob already saved within a pack.
type PackedBlob struct {
	pack.Blob
	PackID backend.ID
}

// Each returns a channel that yields all blobs known to the index. If done is
// closed, the background goroutine terminates. This blocks any modification of
// the index.
func (idx *Index) Each(done chan struct{}) <-chan PackedBlob {
	idx.m.Lock()

	ch := make(chan PackedBlob)

	go func() {
		defer idx.m.Unlock()
		defer func() {
			close(ch)
		}()

		for id, blob := range idx.pack {
			select {
			case <-done:
				return
			case ch <- PackedBlob{
				Blob: pack.Blob{
					ID:     id,
					Offset: blob.offset,
					Type:   blob.tpe,
					Length: uint32(blob.length),
				},
				PackID: *blob.packID,
			}:
			}
		}
	}()

	return ch
}

// Count returns the number of blobs of type t in the index.
func (idx *Index) Count(t pack.BlobType) (n uint) {
	debug.Log("Index.Count", "counting blobs of type %v", t)
	idx.m.Lock()
	defer idx.m.Unlock()

	for id, blob := range idx.pack {
		if blob.tpe == t {
			n++
			debug.Log("Index.Count", "  blob %v counted: %v", id[:8], blob)
		}
	}

	return
}

type packJSON struct {
	ID    backend.ID `json:"id"`
	Blobs []blobJSON `json:"blobs"`
}

type blobJSON struct {
	ID     backend.ID    `json:"id"`
	Type   pack.BlobType `json:"type"`
	Offset uint          `json:"offset"`
	Length uint          `json:"length"`
}

// generatePackList returns a list of packs containing only the index entries
// that selsectFn returned true for. If selectFn is nil, the list contains all
// blobs in the index.
func (idx *Index) generatePackList(selectFn func(indexEntry) bool) ([]*packJSON, error) {
	list := []*packJSON{}
	packs := make(map[backend.ID]*packJSON)

	for id, blob := range idx.pack {
		if selectFn != nil && !selectFn(blob) {
			continue
		}

		debug.Log("Index.generatePackList", "handle blob %q", id[:8])

		if blob.packID.IsNull() {
			debug.Log("Index.generatePackList", "blob %q has no packID! (type %v, offset %v, length %v)",
				id[:8], blob.tpe, blob.offset, blob.length)
			return nil, fmt.Errorf("unable to serialize index: pack for blob %v hasn't been written yet", id)
		}

		// see if pack is already in map
		p, ok := packs[*blob.packID]
		if !ok {
			// else create new pack
			p = &packJSON{ID: *blob.packID}

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

	debug.Log("Index.generatePackList", "done")

	return list, nil
}

// encode writes the JSON serialization of the index filtered by selectFn to enc.
func (idx *Index) encode(w io.Writer, selectFn func(indexEntry) bool) error {
	list, err := idx.generatePackList(func(entry indexEntry) bool {
		return !entry.old
	})
	if err != nil {
		return err
	}

	debug.Log("Index.Encode", "done")

	enc := json.NewEncoder(w)
	return enc.Encode(list)
}

// Encode writes the JSON serialization of the index to the writer w. This
// serialization only contains new blobs added via idx.Store(), not old ones
// introduced via DecodeIndex().
func (idx *Index) Encode(w io.Writer) error {
	debug.Log("Index.Encode", "encoding index")
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.encode(w, func(e indexEntry) bool { return !e.old })
}

// Dump writes the pretty-printed JSON representation of the index to w.
func (idx *Index) Dump(w io.Writer) error {
	debug.Log("Index.Dump", "dumping index")
	idx.m.Lock()
	defer idx.m.Unlock()

	list, err := idx.generatePackList(nil)
	if err != nil {
		return err
	}

	buf, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	_, err = w.Write(append(buf, '\n'))
	if err != nil {
		return err
	}

	debug.Log("Index.Dump", "done")

	return nil
}

// DecodeIndex loads and unserializes an index from rd.
func DecodeIndex(rd io.Reader) (*Index, error) {
	debug.Log("Index.DecodeIndex", "Start decoding index")
	list := []*packJSON{}

	dec := json.NewDecoder(rd)
	err := dec.Decode(&list)
	if err != nil {
		return nil, err
	}

	idx := NewIndex()
	for _, pack := range list {
		for _, blob := range pack.Blobs {
			idx.store(blob.Type, blob.ID, &pack.ID, blob.Offset, blob.Length, true)
		}
	}

	debug.Log("Index.DecodeIndex", "done")
	return idx, err
}
