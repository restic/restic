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

// In large repositories, millions of blobs are stored in the repository
// and restic needs to store an index entry for each blob in memory for
// most operations.
// Hence the index data structure defined here is one of the main contributions
// to the total memory requirements of restic.
//
// We store the index entries in indexMaps. In these maps, entries take 56
// bytes each, plus 8/4 = 2 bytes of unused pointers on average, not counting
// malloc and header struct overhead and ignoring duplicates (those are only
// present in edge cases and are also removed by prune runs).
//
// In the index entries, we need to reference the packID. As one pack may
// contain many blobs the packIDs are saved in a separate array and only the index
// within this array is saved in the indexEntry
//
// We assume on average a minimum of 8 blobs per pack; BP=8.
// (Note that for large files there should be 3 blobs per pack as the average chunk
// size is 1.5 MB and the minimum pack size is 4 MB)
//
// We have the following sizes:
// indexEntry:  56 bytes  (on amd64)
// each packID: 32 bytes
//
// To save N index entries, we therefore need:
// N * (56 + 2) bytes + N * 32 bytes / BP = N * 62 bytes,
// i.e., fewer than 64 bytes per blob in an index.

// Index holds lookup tables for id -> pack.
type Index struct {
	m         sync.Mutex
	byType    [restic.NumBlobTypes]indexMap
	packs     restic.IDs
	treePacks restic.IDs
	// only used by Store, StorePacks does not check for already saved packIDs
	packIDToIndex map[restic.ID]int

	final      bool      // set to true for all indexes read from the backend ("finalized")
	id         restic.ID // set to the ID of the index when it's finalized
	supersedes restic.IDs
	created    time.Time
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		packIDToIndex: make(map[restic.ID]int),
		created:       time.Now(),
	}
}

// addToPacks saves the given pack ID and return the index.
// This procedere allows to use pack IDs which can be easily garbage collected after.
func (idx *Index) addToPacks(id restic.ID) int {
	idx.packs = append(idx.packs, id)
	return len(idx.packs) - 1
}

const maxuint32 = 1<<32 - 1

func (idx *Index) store(packIndex int, blob restic.Blob) {
	// assert that offset and length fit into uint32!
	if blob.Offset > maxuint32 || blob.Length > maxuint32 {
		panic("offset or length does not fit in uint32. You have packs > 4GB!")
	}

	m := &idx.byType[blob.Type]
	m.add(blob.ID, packIndex, uint32(blob.Offset), uint32(blob.Length))
}

// Final returns true iff the index is already written to the repository, it is
// finalized.
func (idx *Index) Final() bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.final
}

const (
	indexMaxBlobs = 50000
	indexMaxAge   = 10 * time.Minute
)

// IndexFull returns true iff the index is "full enough" to be saved as a preliminary index.
var IndexFull = func(idx *Index) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("checking whether index %p is full", idx)

	var blobs uint
	for typ := range idx.byType {
		blobs += idx.byType[typ].len()
	}
	age := time.Now().Sub(idx.created)

	switch {
	case age >= indexMaxAge:
		debug.Log("index %p is old enough", idx, age)
		return true
	case blobs >= indexMaxBlobs:
		debug.Log("index %p has %d blobs", idx, blobs)
		return true
	}

	debug.Log("index %p only has %d blobs and is too young (%v)", idx, blobs, age)
	return false

}

// Store remembers the id and pack in the index.
func (idx *Index) Store(blob restic.PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		panic("store new item in finalized index")
	}

	debug.Log("%v", blob)

	// get packIndex and save if new packID
	packIndex, ok := idx.packIDToIndex[blob.PackID]
	if !ok {
		packIndex = idx.addToPacks(blob.PackID)
		idx.packIDToIndex[blob.PackID] = packIndex
	}

	idx.store(packIndex, blob.Blob)
}

// StorePack remembers the ids of all blobs of a given pack
// in the index
func (idx *Index) StorePack(id restic.ID, blobs []restic.Blob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		panic("store new item in finalized index")
	}

	debug.Log("%v", blobs)
	packIndex := idx.addToPacks(id)

	for _, blob := range blobs {
		idx.store(packIndex, blob)
	}
}

func (idx *Index) toPackedBlob(e *indexEntry, typ restic.BlobType) restic.PackedBlob {
	return restic.PackedBlob{
		Blob: restic.Blob{
			ID:     e.id,
			Type:   typ,
			Length: uint(e.length),
			Offset: uint(e.offset),
		},
		PackID: idx.packs[e.packIndex],
	}
}

// Lookup queries the index for the blob ID and returns a restic.PackedBlob.
func (idx *Index) Lookup(id restic.ID, tpe restic.BlobType) (blobs []restic.PackedBlob, found bool) {
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.byType[tpe].foreachWithID(id, func(e *indexEntry) {
		blobs = append(blobs, idx.toPackedBlob(e, tpe))
	})

	return blobs, len(blobs) > 0
}

// ListPack returns a list of blobs contained in a pack.
func (idx *Index) ListPack(id restic.ID) (list []restic.PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	for typ := range idx.byType {
		m := &idx.byType[typ]
		m.foreach(func(e *indexEntry) bool {
			if idx.packs[e.packIndex] == id {
				list = append(list, idx.toPackedBlob(e, restic.BlobType(typ)))
			}
			return true
		})
	}

	return list
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id restic.ID, tpe restic.BlobType) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.byType[tpe].get(id) != nil
}

// LookupSize returns the length of the plaintext content of the blob with the
// given id.
func (idx *Index) LookupSize(id restic.ID, tpe restic.BlobType) (plaintextLength uint, found bool) {
	idx.m.Lock()
	defer idx.m.Unlock()

	e := idx.byType[tpe].get(id)
	if e == nil {
		return 0, false
	}
	return uint(restic.PlaintextLength(int(e.length))), true
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

		for typ := range idx.byType {
			m := &idx.byType[typ]
			m.foreach(func(e *indexEntry) bool {
				select {
				case <-ctx.Done():
					return false
				case ch <- idx.toPackedBlob(e, restic.BlobType(typ)):
					return true
				}
			})
		}
	}()

	return ch
}

// Packs returns all packs in this index
func (idx *Index) Packs() restic.IDSet {
	idx.m.Lock()
	defer idx.m.Unlock()

	packs := restic.NewIDSet()
	for _, packID := range idx.packs {
		packs.Insert(packID)
	}

	return packs
}

// Count returns the number of blobs of type t in the index.
func (idx *Index) Count(t restic.BlobType) (n uint) {
	debug.Log("counting blobs of type %v", t)
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.byType[t].len()
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

	for typ := range idx.byType {
		m := &idx.byType[typ]
		m.foreach(func(e *indexEntry) bool {
			packID := idx.packs[e.packIndex]
			if packID.IsNull() {
				panic("null pack id")
			}

			debug.Log("handle blob %v", e.id)

			// see if pack is already in map
			p, ok := packs[packID]
			if !ok {
				// else create new pack
				p = &packJSON{ID: packID}

				// and append it to the list and map
				list = append(list, p)
				packs[p.ID] = p
			}

			// add blob
			p.Blobs = append(p.Blobs, blobJSON{
				ID:     e.id,
				Type:   restic.BlobType(typ),
				Offset: uint(e.offset),
				Length: uint(e.length),
			})

			return true
		})
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

// Finalize sets the index to final.
func (idx *Index) Finalize() {
	debug.Log("finalizing index")
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.final = true
	// clear packIDToIndex as no more elements will be added
	idx.packIDToIndex = nil
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

	debug.Log("ID set to %v", id)
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
		packID := idx.addToPacks(pack.ID)

		for _, blob := range pack.Blobs {
			idx.store(packID, restic.Blob{
				Type:   blob.Type,
				ID:     blob.ID,
				Offset: blob.Offset,
				Length: blob.Length,
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
		packID := idx.addToPacks(pack.ID)

		for _, blob := range pack.Blobs {
			idx.store(packID, restic.Blob{
				Type:   blob.Type,
				ID:     blob.ID,
				Offset: blob.Offset,
				Length: blob.Length,
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
func LoadIndexWithDecoder(ctx context.Context, repo restic.Repository, buf []byte, id restic.ID, fn func([]byte) (*Index, error)) (*Index, []byte, error) {
	debug.Log("Loading index %v", id)

	buf, err := repo.LoadAndDecrypt(ctx, buf[:0], restic.IndexFile, id)
	if err != nil {
		return nil, buf[:0], err
	}

	idx, err := fn(buf)
	if err != nil {
		debug.Log("error while decoding index %v: %v", id, err)
		return nil, buf[:0], err
	}

	idx.id = id

	return idx, buf, nil
}
