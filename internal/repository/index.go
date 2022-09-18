package repository

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/restic/restic/internal/crypto"
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
	m          sync.Mutex
	byType     [restic.NumBlobTypes]indexMap
	packs      restic.IDs
	mixedPacks restic.IDSet

	final      bool       // set to true for all indexes read from the backend ("finalized")
	ids        restic.IDs // set to the IDs of the contained finalized indexes
	supersedes restic.IDs
	created    time.Time
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		mixedPacks: restic.NewIDSet(),
		created:    time.Now(),
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
	if blob.Offset > maxuint32 || blob.Length > maxuint32 || blob.UncompressedLength > maxuint32 {
		panic("offset or length does not fit in uint32. You have packs > 4GB!")
	}

	m := &idx.byType[blob.Type]
	m.add(blob.ID, packIndex, uint32(blob.Offset), uint32(blob.Length), uint32(blob.UncompressedLength))
}

// Final returns true iff the index is already written to the repository, it is
// finalized.
func (idx *Index) Final() bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.final
}

const (
	indexMaxBlobs           = 50000
	indexMaxBlobsCompressed = 3 * indexMaxBlobs
	indexMaxAge             = 10 * time.Minute
)

// IndexFull returns true iff the index is "full enough" to be saved as a preliminary index.
var IndexFull = func(idx *Index, compress bool) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("checking whether index %p is full", idx)

	var blobs uint
	for typ := range idx.byType {
		blobs += idx.byType[typ].len()
	}
	age := time.Since(idx.created)
	var maxBlobs uint
	if compress {
		maxBlobs = indexMaxBlobsCompressed
	} else {
		maxBlobs = indexMaxBlobs
	}

	switch {
	case age >= indexMaxAge:
		debug.Log("index %p is old enough", idx, age)
		return true
	case blobs >= maxBlobs:
		debug.Log("index %p has %d blobs", idx, blobs)
		return true
	}

	debug.Log("index %p only has %d blobs and is too young (%v)", idx, blobs, age)
	return false

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

func (idx *Index) toPackedBlob(e *indexEntry, t restic.BlobType) restic.PackedBlob {
	return restic.PackedBlob{
		Blob: restic.Blob{
			BlobHandle: restic.BlobHandle{
				ID:   e.id,
				Type: t},
			Length:             uint(e.length),
			Offset:             uint(e.offset),
			UncompressedLength: uint(e.uncompressedLength),
		},
		PackID: idx.packs[e.packIndex],
	}
}

// Lookup queries the index for the blob ID and returns all entries including
// duplicates. Adds found entries to blobs and returns the result.
func (idx *Index) Lookup(bh restic.BlobHandle, pbs []restic.PackedBlob) []restic.PackedBlob {
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.byType[bh.Type].foreachWithID(bh.ID, func(e *indexEntry) {
		pbs = append(pbs, idx.toPackedBlob(e, bh.Type))
	})

	return pbs
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(bh restic.BlobHandle) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.byType[bh.Type].get(bh.ID) != nil
}

// LookupSize returns the length of the plaintext content of the blob with the
// given id.
func (idx *Index) LookupSize(bh restic.BlobHandle) (plaintextLength uint, found bool) {
	idx.m.Lock()
	defer idx.m.Unlock()

	e := idx.byType[bh.Type].get(bh.ID)
	if e == nil {
		return 0, false
	}
	if e.uncompressedLength != 0 {
		return uint(e.uncompressedLength), true
	}
	return uint(crypto.PlaintextLength(int(e.length))), true
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

type EachByPackResult struct {
	PackID restic.ID
	Blobs  []restic.Blob
}

// EachByPack returns a channel that yields all blobs known to the index
// grouped by packID but ignoring blobs with a packID in packPlacklist for
// finalized indexes.
// This filtering is used when rebuilding the index where we need to ignore packs
// from the finalized index which have been re-read into a non-finalized index.
// When the  context is cancelled, the background goroutine
// terminates. This blocks any modification of the index.
func (idx *Index) EachByPack(ctx context.Context, packBlacklist restic.IDSet) <-chan EachByPackResult {
	idx.m.Lock()

	ch := make(chan EachByPackResult)

	go func() {
		defer idx.m.Unlock()
		defer func() {
			close(ch)
		}()

		byPack := make(map[restic.ID][][]*indexEntry)

		for typ := range idx.byType {
			m := &idx.byType[typ]
			m.foreach(func(e *indexEntry) bool {
				packID := idx.packs[e.packIndex]
				if !idx.final || !packBlacklist.Has(packID) {
					if _, ok := byPack[packID]; !ok {
						byPack[packID] = make([][]*indexEntry, restic.NumBlobTypes)
					}
					byPack[packID][typ] = append(byPack[packID][typ], e)
				}
				return true
			})
		}

		for packID, packByType := range byPack {
			var result EachByPackResult
			result.PackID = packID
			for typ, pack := range packByType {
				for _, e := range pack {
					result.Blobs = append(result.Blobs, idx.toPackedBlob(e, restic.BlobType(typ)).Blob)
				}
			}
			// allow GC once entry is no longer necessary
			byPack[packID] = nil
			select {
			case <-ctx.Done():
				return
			case ch <- result:
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
	for _, packID := range idx.packs {
		packs.Insert(packID)
	}

	return packs
}

type packJSON struct {
	ID    restic.ID  `json:"id"`
	Blobs []blobJSON `json:"blobs"`
}

type blobJSON struct {
	ID                 restic.ID       `json:"id"`
	Type               restic.BlobType `json:"type"`
	Offset             uint            `json:"offset"`
	Length             uint            `json:"length"`
	UncompressedLength uint            `json:"uncompressed_length,omitempty"`
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
				ID:                 e.id,
				Type:               restic.BlobType(typ),
				Offset:             uint(e.offset),
				Length:             uint(e.length),
				UncompressedLength: uint(e.uncompressedLength),
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
}

// IDs returns the IDs of the index, if available. If the index is not yet
// finalized, an error is returned.
func (idx *Index) IDs() (restic.IDs, error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return nil, errors.New("index not finalized")
	}

	return idx.ids, nil
}

// SetID sets the ID the index has been written to. This requires that
// Finalize() has been called before, otherwise an error is returned.
func (idx *Index) SetID(id restic.ID) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return errors.New("index is not final")
	}

	if len(idx.ids) > 0 {
		return errors.New("ID already set")
	}

	debug.Log("ID set to %v", id)
	idx.ids = append(idx.ids, id)

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

// MixedPacks returns an IDSet that contain packs which have mixed blobs.
func (idx *Index) MixedPacks() restic.IDSet {
	return idx.mixedPacks
}

// merge() merges indexes, i.e. idx.merge(idx2) merges the contents of idx2 into idx.
// During merging exact duplicates are removed;  idx2 is not changed by this method.
func (idx *Index) merge(idx2 *Index) error {
	idx.m.Lock()
	defer idx.m.Unlock()
	idx2.m.Lock()
	defer idx2.m.Unlock()

	if !idx2.final {
		return errors.New("index to merge is not final")
	}

	packlen := len(idx.packs)
	// first append packs as they might be accessed when looking for duplicates below
	idx.packs = append(idx.packs, idx2.packs...)

	// copy all index entries of idx2 to idx
	for typ := range idx2.byType {
		m2 := &idx2.byType[typ]
		m := &idx.byType[typ]

		// helper func to test if identical entry is contained in idx
		hasIdenticalEntry := func(e2 *indexEntry) (found bool) {
			m.foreachWithID(e2.id, func(e *indexEntry) {
				b := idx.toPackedBlob(e, restic.BlobType(typ))
				b2 := idx2.toPackedBlob(e2, restic.BlobType(typ))
				if b == b2 {
					found = true
				}
			})
			return found
		}

		m2.foreach(func(e2 *indexEntry) bool {
			if !hasIdenticalEntry(e2) {
				// packIndex needs to be changed as idx2.pack was appended to idx.pack, see above
				m.add(e2.id, e2.packIndex+packlen, e2.offset, e2.length, e2.uncompressedLength)
			}
			return true
		})
	}

	idx.mixedPacks.Merge(idx2.mixedPacks)
	idx.ids = append(idx.ids, idx2.ids...)
	idx.supersedes = append(idx.supersedes, idx2.supersedes...)

	return nil
}

// isErrOldIndex returns true if the error may be caused by an old index
// format.
func isErrOldIndex(err error) bool {
	e, ok := err.(*json.UnmarshalTypeError)
	return ok && e.Value == "array"
}

// DecodeIndex unserializes an index from buf.
func DecodeIndex(buf []byte, id restic.ID) (idx *Index, oldFormat bool, err error) {
	debug.Log("Start decoding index")
	idxJSON := &jsonIndex{}

	err = json.Unmarshal(buf, idxJSON)
	if err != nil {
		debug.Log("Error %v", err)

		if isErrOldIndex(err) {
			debug.Log("index is probably old format, trying that")
			idx, err = decodeOldIndex(buf)
			return idx, err == nil, err
		}

		return nil, false, errors.Wrap(err, "DecodeIndex")
	}

	idx = NewIndex()
	for _, pack := range idxJSON.Packs {
		var data, tree bool
		packID := idx.addToPacks(pack.ID)

		for _, blob := range pack.Blobs {
			idx.store(packID, restic.Blob{
				BlobHandle: restic.BlobHandle{
					Type: blob.Type,
					ID:   blob.ID},
				Offset:             blob.Offset,
				Length:             blob.Length,
				UncompressedLength: blob.UncompressedLength,
			})

			switch blob.Type {
			case restic.DataBlob:
				data = true
			case restic.TreeBlob:
				tree = true
			}
		}

		if data && tree {
			idx.mixedPacks.Insert(pack.ID)
		}
	}
	idx.supersedes = idxJSON.Supersedes
	idx.ids = append(idx.ids, id)
	idx.final = true

	debug.Log("done")
	return idx, false, nil
}

// DecodeOldIndex loads and unserializes an index in the old format from rd.
func decodeOldIndex(buf []byte) (idx *Index, err error) {
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
				BlobHandle: restic.BlobHandle{
					Type: blob.Type,
					ID:   blob.ID},
				Offset: blob.Offset,
				Length: blob.Length,
				// no compressed length in the old index format
			})

			switch blob.Type {
			case restic.DataBlob:
				data = true
			case restic.TreeBlob:
				tree = true
			}
		}

		if data && tree {
			idx.mixedPacks.Insert(pack.ID)
		}
	}
	idx.final = true

	debug.Log("done")
	return idx, nil
}
