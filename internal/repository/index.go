package repository

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"
)

// Index holds a lookup table for id -> pack.
type Index struct {
	m         sync.Mutex
	pack      map[restic.BlobHandle][]indexEntry
	treePacks restic.IDs

	final      bool      // set to true for all indexes read from the backend ("finalized")
	id         restic.ID // set to the ID of the index when it's finalized
	supersedes restic.IDs
	created    time.Time
}

type indexEntry struct {
	packID restic.ID
	offset uint
	length uint
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		pack:    make(map[restic.BlobHandle][]indexEntry),
		created: time.Now(),
	}
}

func (idx *Index) store(blob restic.PackedBlob) {
	newEntry := indexEntry{
		packID: blob.PackID,
		offset: blob.Offset,
		length: blob.Length,
	}
	h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
	idx.pack[h] = append(idx.pack[h], newEntry)
}

// Final returns true iff the index is already written to the repository, it is
// finalized.
func (idx *Index) Final() bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.final
}

const (
	indexMinBlobs = 20
	indexMaxBlobs = 2000
	indexMinAge   = 2 * time.Minute
	indexMaxAge   = 15 * time.Minute
)

// IndexFull returns true iff the index is "full enough" to be saved as a preliminary index.
var IndexFull = func(idx *Index) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("checking whether index %p is full", idx)

	packs := len(idx.pack)
	age := time.Now().Sub(idx.created)

	if age > indexMaxAge {
		debug.Log("index %p is old enough", idx, age)
		return true
	}

	if packs < indexMinBlobs || age < indexMinAge {
		debug.Log("index %p only has %d packs or is too young (%v)", idx, packs, age)
		return false
	}

	if packs > indexMaxBlobs {
		debug.Log("index %p has %d packs", idx, packs)
		return true
	}

	debug.Log("index %p is not full", idx)
	return false
}

// Store remembers the id and pack in the index. An existing entry will be
// silently overwritten.
func (idx *Index) Store(blob restic.PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		panic("store new item in finalized index")
	}

	debug.Log("%v", blob)

	idx.store(blob)
}

// Lookup queries the index for the blob ID and returns a restic.PackedBlob.
func (idx *Index) Lookup(id restic.ID, tpe restic.BlobType) (blobs []restic.PackedBlob, err error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	h := restic.BlobHandle{ID: id, Type: tpe}

	if packs, ok := idx.pack[h]; ok {
		blobs = make([]restic.PackedBlob, 0, len(packs))

		for _, p := range packs {
			debug.Log("id %v found in pack %v at %d, length %d",
				id.Str(), p.packID.Str(), p.offset, p.length)

			blob := restic.PackedBlob{
				Blob: restic.Blob{
					Type:   tpe,
					Length: p.length,
					ID:     id,
					Offset: p.offset,
				},
				PackID: p.packID,
			}

			blobs = append(blobs, blob)
		}

		return blobs, nil
	}

	debug.Log("id %v not found", id.Str())
	return nil, errors.Errorf("id %v not found in index", id)
}

// ListPack returns a list of blobs contained in a pack.
func (idx *Index) ListPack(id restic.ID) (list []restic.PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	for h, packList := range idx.pack {
		for _, entry := range packList {
			if entry.packID == id {
				list = append(list, restic.PackedBlob{
					Blob: restic.Blob{
						ID:     h.ID,
						Type:   h.Type,
						Length: entry.length,
						Offset: entry.offset,
					},
					PackID: entry.packID,
				})
			}
		}
	}

	return list
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id restic.ID, tpe restic.BlobType) bool {
	_, err := idx.Lookup(id, tpe)
	if err == nil {
		return true
	}

	return false
}

// LookupSize returns the length of the plaintext content of the blob with the
// given id.
func (idx *Index) LookupSize(id restic.ID, tpe restic.BlobType) (plaintextLength uint, err error) {
	blobs, err := idx.Lookup(id, tpe)
	if err != nil {
		return 0, err
	}

	return uint(restic.PlaintextLength(int(blobs[0].Length))), nil
}

// Supersedes returns the list of indexes this index supersedes, if any.
func (idx *Index) Supersedes() restic.IDs {
	return idx.supersedes
}

// AddToSupersedes adds the ids to the list of indexes superseded by this
// index. If the index has already been finalized, an error is returned.
func (idx *Index) AddToSupersedes(ids ...restic.ID) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		return errors.New("index already finalized")
	}

	idx.supersedes = append(idx.supersedes, ids...)
	return nil
}

// Each returns a channel that yields all blobs known to the index. When the
// context is cancelled, the background goroutine terminates. This blocks any
// modification of the index.
func (idx *Index) Each(ctx context.Context) <-chan restic.PackedBlob {
	idx.m.Lock()

	ch := make(chan restic.PackedBlob)

	go func() {
		defer idx.m.Unlock()
		defer func() {
			close(ch)
		}()

		for h, packs := range idx.pack {
			for _, blob := range packs {
				select {
				case <-ctx.Done():
					return
				case ch <- restic.PackedBlob{
					Blob: restic.Blob{
						ID:     h.ID,
						Type:   h.Type,
						Offset: blob.offset,
						Length: blob.length,
					},
					PackID: blob.packID,
				}:
				}
			}
		}
	}()

	return ch
}

// Packs returns all packs in this index
func (idx *Index) Packs() restic.IDSet {
	idx.m.Lock()
	defer idx.m.Unlock()

	packs := restic.NewIDSet()
	for _, list := range idx.pack {
		for _, entry := range list {
			packs.Insert(entry.packID)
		}
	}

	return packs
}

// Count returns the number of blobs of type t in the index.
func (idx *Index) Count(t restic.BlobType) (n uint) {
	debug.Log("counting blobs of type %v", t)
	idx.m.Lock()
	defer idx.m.Unlock()

	for h, list := range idx.pack {
		if h.Type != t {
			continue
		}

		n += uint(len(list))
	}

	return
}

type packJSON struct {
	ID    restic.ID  `json:"id"`
	Blobs []blobJSON `json:"blobs"`
}

type blobJSON struct {
	ID     restic.ID       `json:"id"`
	Type   restic.BlobType `json:"type"`
	Offset uint            `json:"offset"`
	Length uint            `json:"length"`
}

// generatePackList returns a list of packs.
func (idx *Index) generatePackList() ([]*packJSON, error) {
	list := []*packJSON{}
	packs := make(map[restic.ID]*packJSON)

	for h, packedBlobs := range idx.pack {
		for _, blob := range packedBlobs {
			if blob.packID.IsNull() {
				panic("null pack id")
			}

			debug.Log("handle blob %v", h)

			if blob.packID.IsNull() {
				debug.Log("blob %v has no packID! (offset %v, length %v)",
					h, blob.offset, blob.length)
				return nil, errors.Errorf("unable to serialize index: pack for blob %v hasn't been written yet", h)
			}

			// see if pack is already in map
			p, ok := packs[blob.packID]
			if !ok {
				// else create new pack
				p = &packJSON{ID: blob.packID}

				// and append it to the list and map
				list = append(list, p)
				packs[p.ID] = p
			}

			// add blob
			p.Blobs = append(p.Blobs, blobJSON{
				ID:     h.ID,
				Type:   h.Type,
				Offset: blob.offset,
				Length: blob.length,
			})
		}
	}

	debug.Log("done")

	return list, nil
}

type jsonIndex struct {
	Supersedes restic.IDs  `json:"supersedes,omitempty"`
	Packs      []*packJSON `json:"packs"`
}

// Encode writes the JSON serialization of the index to the writer w.
func (idx *Index) Encode(w io.Writer) error {
	debug.Log("encoding index")
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.encode(w)
}

// encode writes the JSON serialization of the index to the writer w.
func (idx *Index) encode(w io.Writer) error {
	debug.Log("encoding index")

	list, err := idx.generatePackList()
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	idxJSON := jsonIndex{
		Supersedes: idx.supersedes,
		Packs:      list,
	}
	return enc.Encode(idxJSON)
}

// Finalize sets the index to final and writes the JSON serialization to w.
func (idx *Index) Finalize(w io.Writer) error {
	debug.Log("encoding index")
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.final = true

	return idx.encode(w)
}

// ID returns the ID of the index, if available. If the index is not yet
// finalized, an error is returned.
func (idx *Index) ID() (restic.ID, error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return restic.ID{}, errors.New("index not finalized")
	}

	return idx.id, nil
}

// SetID sets the ID the index has been written to. This requires that
// Finalize() has been called before, otherwise an error is returned.
func (idx *Index) SetID(id restic.ID) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return errors.New("index is not final")
	}

	if !idx.id.IsNull() {
		return errors.New("ID already set")
	}

	debug.Log("ID set to %v", id.Str())
	idx.id = id

	return nil
}

// Dump writes the pretty-printed JSON representation of the index to w.
func (idx *Index) Dump(w io.Writer) error {
	debug.Log("dumping index")
	idx.m.Lock()
	defer idx.m.Unlock()

	list, err := idx.generatePackList()
	if err != nil {
		return err
	}

	outer := jsonIndex{
		Supersedes: idx.Supersedes(),
		Packs:      list,
	}

	buf, err := json.MarshalIndent(outer, "", "  ")
	if err != nil {
		return err
	}

	_, err = w.Write(append(buf, '\n'))
	if err != nil {
		return errors.Wrap(err, "Write")
	}

	debug.Log("done")

	return nil
}

// TreePacks returns a list of packs that contain only tree blobs.
func (idx *Index) TreePacks() restic.IDs {
	return idx.treePacks
}

// isErrOldIndex returns true if the error may be caused by an old index
// format.
func isErrOldIndex(err error) bool {
	if e, ok := err.(*json.UnmarshalTypeError); ok && e.Value == "array" {
		return true
	}

	return false
}

// ErrOldIndexFormat means an index with the old format was detected.
var ErrOldIndexFormat = errors.New("index has old format")

// DecodeIndex loads and unserializes an index from rd.
func DecodeIndex(buf []byte) (idx *Index, err error) {
	debug.Log("Start decoding index")
	idxJSON := &jsonIndex{}

	err = json.Unmarshal(buf, idxJSON)
	if err != nil {
		debug.Log("Error %v", err)

		if isErrOldIndex(err) {
			debug.Log("index is probably old format, trying that")
			err = ErrOldIndexFormat
		}

		return nil, errors.Wrap(err, "Decode")
	}

	idx = NewIndex()
	for _, pack := range idxJSON.Packs {
		var data, tree bool

		for _, blob := range pack.Blobs {
			idx.store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   blob.Type,
					ID:     blob.ID,
					Offset: blob.Offset,
					Length: blob.Length,
				},
				PackID: pack.ID,
			})

			switch blob.Type {
			case restic.DataBlob:
				data = true
			case restic.TreeBlob:
				tree = true
			}
		}

		if !data && tree {
			idx.treePacks = append(idx.treePacks, pack.ID)
		}
	}
	idx.supersedes = idxJSON.Supersedes
	idx.final = true

	debug.Log("done")
	return idx, nil
}

// DecodeOldIndex loads and unserializes an index in the old format from rd.
func DecodeOldIndex(buf []byte) (idx *Index, err error) {
	debug.Log("Start decoding old index")
	list := []*packJSON{}

	err = json.Unmarshal(buf, &list)
	if err != nil {
		debug.Log("Error %#v", err)
		return nil, errors.Wrap(err, "Decode")
	}

	idx = NewIndex()
	for _, pack := range list {
		var data, tree bool

		for _, blob := range pack.Blobs {
			idx.store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   blob.Type,
					ID:     blob.ID,
					Offset: blob.Offset,
					Length: blob.Length,
				},
				PackID: pack.ID,
			})

			switch blob.Type {
			case restic.DataBlob:
				data = true
			case restic.TreeBlob:
				tree = true
			}
		}

		if !data && tree {
			idx.treePacks = append(idx.treePacks, pack.ID)
		}
	}
	idx.final = true

	debug.Log("done")
	return idx, nil
}

// LoadIndexWithDecoder loads the index and decodes it with fn.
func LoadIndexWithDecoder(ctx context.Context, repo restic.Repository, id restic.ID, fn func([]byte) (*Index, error)) (idx *Index, err error) {
	debug.Log("Loading index %v", id.Str())

	buf, err := repo.LoadAndDecrypt(ctx, restic.IndexFile, id)
	if err != nil {
		return nil, err
	}

	idx, err = fn(buf)
	if err != nil {
		debug.Log("error while decoding index %v: %v", id, err)
		return nil, err
	}

	idx.id = id

	return idx, nil
}
