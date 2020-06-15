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
// We use two maps to store each index entry.
// The first map stores the first entry of a blobtype/blobID
// The key of the map is a BlobHandle
// The entries are the actual index entries.
// In the second map we store duplicate index entries, i.e. entries with same
// blobtype/blobID
// In the index entries, we need to reference the packID. As one pack may
// contain many blobs the packIDs are saved in a separate array and only the index
// within this array is saved in the indexEntry
//
// To compute the needed amount of memory, we need some assumptions.
// Maps need an overhead of allocated but not needed elements.
// For computations, we assume an overhead of 50% and use OF=1.5 (overhead factor)
// As duplicates are only present in edge cases and are also removed by prune runs,
// we assume that there are no significant duplicates and omit them in the calculations.
// Moreover we asssume on average a minimum of 8 blobs per pack; BP=8
// (Note that for large files there should be 3 blobs per pack as the average chunk
// size is 1.5 MB and the minimum pack size is 4 MB)
//
// We have the following sizes:
// key: 32 + 1 = 33 bytes
// indexEntry:  8 + 4 + 4 = 16 bytes
// each packID: 32 bytes
//
// To save N index entries, we therefore need:
// N * OF * (33 + 16) bytes + N * 32 bytes / BP = N * 78 bytes

// Index holds lookup tables for id -> pack.
type Index struct {
	m          sync.Mutex
	blob       map[restic.BlobHandle]indexEntry
	duplicates map[restic.BlobHandle][]indexEntry
	packs      restic.IDs
	treePacks  restic.IDs
	// only used by Store, StorePacks does not check for already saved packIDs
	packIDToIndex map[restic.ID]int

	option restic.IndexOption
	// only used with option DataIDsOnly
	dataIDs restic.IDSet
	// only used when option != FullIndex
	repo *Repository

	final      bool      // set to true for all indexes read from the backend ("finalized")
	id         restic.ID // set to the ID of the index when it's finalized
	supersedes restic.IDs
	created    time.Time
}

type indexEntry struct {
	// only save index do packs; i.e. packs[packindex] yields the packID
	packIndex int
	offset    uint32
	length    uint32
}

// NewIndex returns a new index.
func NewIndex(option restic.IndexOption) *Index {
	var dataIDs restic.IDSet
	if option == restic.IndexOptionDataIDsOnly {
		dataIDs = restic.NewIDSet()
	}
	return &Index{
		blob:          make(map[restic.BlobHandle]indexEntry),
		duplicates:    make(map[restic.BlobHandle][]indexEntry),
		packIDToIndex: make(map[restic.ID]int),
		option:        option,
		dataIDs:       dataIDs,
		created:       time.Now(),
	}
}

func (idx *Index) assertOption() {
	if idx.option != restic.IndexOptionFull {
		panic("not implemented!")
	}
}

// withDuplicates returns the list of all entries for the given blob handle
func (idx *Index) withDuplicates(h restic.BlobHandle, entry indexEntry) []indexEntry {
	entries, ok := idx.duplicates[h]
	if ok {
		all := make([]indexEntry, len(entries)+1)
		all[0] = entry
		copy(all[1:], entries)
		return all
	}

	return []indexEntry{entry}
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
	newEntry := indexEntry{
		packIndex: packIndex,
		offset:    uint32(blob.Offset),
		length:    uint32(blob.Length),
	}
	h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
	if _, ok := idx.blob[h]; ok {
		idx.duplicates[h] = append(idx.duplicates[h], newEntry)
	} else {
		idx.blob[h] = newEntry
	}
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

	blobs := len(idx.blob)
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

	if blob.Type == restic.DataBlob {
		switch idx.option {
		case restic.IndexOptionDataIDsOnly:
			idx.dataIDs.Insert(blob.ID)
			return
		case restic.IndexOptionNoData:
			panic("not implemented!")
		}
	}

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

	idx.assertOption()

	if idx.final {
		panic("store new item in finalized index")
	}

	debug.Log("%v", blobs)
	packIndex := idx.addToPacks(id)

	for _, blob := range blobs {
		idx.store(packIndex, blob)
	}
}

// ListPack returns a list of blobs contained in a pack.
func (idx *Index) indexEntryToPackedBlob(h restic.BlobHandle, entry indexEntry) restic.PackedBlob {
	return restic.PackedBlob{
		Blob: restic.Blob{
			ID:     h.ID,
			Type:   h.Type,
			Length: uint(entry.length),
			Offset: uint(entry.offset),
		},
		PackID: idx.packs[entry.packIndex],
	}
}

// Lookup queries the index for the blob ID and returns a restic.PackedBlob.
func (idx *Index) Lookup(id restic.ID, tpe restic.BlobType) (blobs []restic.PackedBlob, found bool) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if tpe == restic.DataBlob {
		idx.assertOption()
	}

	h := restic.BlobHandle{ID: id, Type: tpe}

	blob, ok := idx.blob[h]
	if ok {
		blobList := idx.withDuplicates(h, blob)
		blobs = make([]restic.PackedBlob, 0, len(blobList))

		for _, p := range blobList {
			blobs = append(blobs, idx.indexEntryToPackedBlob(h, p))
		}

		return blobs, true
	}

	return nil, false
}

// ListPack returns a list of blobs contained in a pack.
func (idx *Index) ListPack(id restic.ID) (list []restic.PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.assertOption()

	for h, entry := range idx.blob {
		for _, blob := range idx.withDuplicates(h, entry) {
			if idx.packs[blob.packIndex] == id {
				list = append(list, idx.indexEntryToPackedBlob(h, blob))
			}
		}
	}

	return list
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id restic.ID, tpe restic.BlobType) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	if tpe == restic.DataBlob {
		switch idx.option {
		case restic.IndexOptionDataIDsOnly:
			return idx.dataIDs.Has(id)
		case restic.IndexOptionNoData:
			panic("not implemented!")
		}
	}

	h := restic.BlobHandle{ID: id, Type: tpe}
	_, ok := idx.blob[h]
	return ok
}

// LookupSize returns the length of the plaintext content of the blob with the
// given id.
func (idx *Index) LookupSize(id restic.ID, tpe restic.BlobType) (plaintextLength uint, found bool) {
	blobs, found := idx.Lookup(id, tpe)
	if !found {
		return 0, found
	}

	return uint(restic.PlaintextLength(int(blobs[0].Length))), true
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
func (idx *Index) Each(ctx context.Context) (<-chan restic.PackedBlob, error) {
	idx.m.Lock()

	if idx.option != restic.IndexOptionFull {
		// reload current index file!
		id, err := idx.ID()
		if err != nil {
			return nil, err
		}
		// TODO: Even better would be to load JSON and directly loop over it in a gofunc
		newIdx, err := LoadIndex(ctx, idx.repo, id, restic.IndexOptionFull)
		if err != nil {
			return nil, err
		}
		return newIdx.Each(ctx)
	}

	ch := make(chan restic.PackedBlob)

	go func() {
		defer idx.m.Unlock()
		defer func() {
			close(ch)
		}()

		for h, entry := range idx.blob {
			for _, blob := range idx.withDuplicates(h, entry) {
				select {
				case <-ctx.Done():
					return
				case ch <- idx.indexEntryToPackedBlob(h, blob):
				}
			}
		}
	}()

	return ch, nil
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

	idx.assertOption()

	for h := range idx.blob {
		if h.Type != t {
			continue
		}
		n++
	}
	for h, dups := range idx.duplicates {
		if h.Type != t {
			continue
		}
		n += uint(len(dups))
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
	idx.assertOption()
	list := []*packJSON{}
	packs := make(map[restic.ID]*packJSON)

	for h, entry := range idx.blob {
		for _, blob := range idx.withDuplicates(h, entry) {
			packID := idx.packs[blob.packIndex]
			if packID.IsNull() {
				panic("null pack id")
			}

			debug.Log("handle blob %v", h)

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
				ID:     h.ID,
				Type:   h.Type,
				Offset: uint(blob.offset),
				Length: uint(blob.length),
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
func DecodeIndex(idx *Index, buf []byte) error {
	debug.Log("Start decoding index")
	idxJSON := &jsonIndex{}

	err := json.Unmarshal(buf, idxJSON)
	if err != nil {
		debug.Log("Error %v", err)

		if isErrOldIndex(err) {
			debug.Log("index is probably old format, trying that")
			err = ErrOldIndexFormat
		}

		return errors.Wrap(err, "Decode")
	}

	idx.supersedes = idxJSON.Supersedes

	return DecodePacklist(idxJSON.Packs, idx)
}

// DecodeOldIndex loads and unserializes an index in the old format from rd.
func DecodeOldIndex(idx *Index, buf []byte) error {
	debug.Log("Start decoding old index")
	list := []*packJSON{}

	err := json.Unmarshal(buf, &list)
	if err != nil {
		debug.Log("Error %#v", err)
		return errors.Wrap(err, "Decode")
	}

	return DecodePacklist(list, idx)
}

func DecodePacklist(list []*packJSON, idx *Index) error {
	for _, pack := range list {
		var data, tree bool
		packID := idx.addToPacks(pack.ID)

		for _, blob := range pack.Blobs {
			switch blob.Type {
			case restic.DataBlob:
				data = true
				if idx.option == restic.IndexOptionDataIDsOnly {
					idx.dataIDs.Insert(blob.ID)
					continue
				}
			case restic.TreeBlob:
				tree = true
			}

			idx.store(packID, restic.Blob{
				Type:   blob.Type,
				ID:     blob.ID,
				Offset: blob.Offset,
				Length: blob.Length,
			})
		}

		if !data && tree {
			idx.treePacks = append(idx.treePacks, pack.ID)
		}
	}
	idx.final = true

	debug.Log("done")
	return nil
}

// LoadIndexWithDecoder loads the index and decodes it with fn.
func LoadIndexWithDecoder(ctx context.Context, repo restic.Repository, buf []byte, id restic.ID, option restic.IndexOption, fn func(*Index, []byte) error) (*Index, []byte, error) {
	debug.Log("Loading index %v", id)

	buf, err := repo.LoadAndDecrypt(ctx, buf[:0], restic.IndexFile, id)
	if err != nil {
		return nil, buf[:0], err
	}

	idx := NewIndex(option)
	err = fn(idx, buf)
	if err != nil {
		debug.Log("error while decoding index %v: %v", id, err)
		return nil, buf[:0], err
	}

	idx.id = id

	return idx, buf, nil
}
