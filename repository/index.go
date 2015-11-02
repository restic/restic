package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// Index holds a lookup table for id -> pack.
type Index struct {
	m    sync.Mutex
	pack map[backend.ID]indexEntry

	final      bool       // set to true for all indexes read from the backend ("finalized")
	id         backend.ID // set to the ID of the index when it's finalized
	supersedes backend.IDs
	created    time.Time
}

type indexEntry struct {
	tpe    pack.BlobType
	packID backend.ID
	offset uint
	length uint
}

// NewIndex returns a new index.
func NewIndex() *Index {
	return &Index{
		pack:    make(map[backend.ID]indexEntry),
		created: time.Now(),
	}
}

func (idx *Index) store(t pack.BlobType, id backend.ID, pack backend.ID, offset, length uint) {
	idx.pack[id] = indexEntry{
		tpe:    t,
		packID: pack,
		offset: offset,
		length: length,
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
	indexMinBlobs = 20
	indexMaxBlobs = 2000
	indexMinAge   = 2 * time.Minute
	indexMaxAge   = 15 * time.Minute
)

// IndexFull returns true iff the index is "full enough" to be saved as a preliminary index.
var IndexFull = func(idx *Index) bool {
	idx.m.Lock()
	defer idx.m.Unlock()

	debug.Log("Index.Full", "checking whether index %p is full", idx)

	packs := len(idx.pack)
	age := time.Now().Sub(idx.created)

	if age > indexMaxAge {
		debug.Log("Index.Full", "index %p is old enough", idx, age)
		return true
	}

	if packs < indexMinBlobs || age < indexMinAge {
		debug.Log("Index.Full", "index %p only has %d packs or is too young (%v)", idx, packs, age)
		return false
	}

	if packs > indexMaxBlobs {
		debug.Log("Index.Full", "index %p has %d packs", idx, packs)
		return true
	}

	debug.Log("Index.Full", "index %p is not full", idx)
	return false
}

// Store remembers the id and pack in the index. An existing entry will be
// silently overwritten.
func (idx *Index) Store(t pack.BlobType, id backend.ID, pack backend.ID, offset, length uint) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		panic("store new item in finalized index")
	}

	debug.Log("Index.Store", "pack %v contains id %v (%v), offset %v, length %v",
		pack.Str(), id.Str(), t, offset, length)

	idx.store(t, id, pack, offset, length)
}

// Lookup queries the index for the blob ID and returns a PackedBlob.
func (idx *Index) Lookup(id backend.ID) (pb PackedBlob, err error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if p, ok := idx.pack[id]; ok {
		debug.Log("Index.Lookup", "id %v found in pack %v at %d, length %d",
			id.Str(), p.packID.Str(), p.offset, p.length)

		pb := PackedBlob{
			Type:   p.tpe,
			Length: p.length,
			ID:     id,
			Offset: p.offset,
			PackID: p.packID,
		}
		return pb, nil
	}

	debug.Log("Index.Lookup", "id %v not found", id.Str())
	return PackedBlob{}, fmt.Errorf("id %v not found in index", id)
}

// ListPack returns a list of blobs contained in a pack.
func (idx *Index) ListPack(id backend.ID) (list []PackedBlob) {
	idx.m.Lock()
	defer idx.m.Unlock()

	for blobID, entry := range idx.pack {
		if entry.packID == id {
			list = append(list, PackedBlob{
				ID:     blobID,
				Type:   entry.tpe,
				Length: entry.length,
				Offset: entry.offset,
				PackID: entry.packID,
			})
		}
	}

	return list
}

// Has returns true iff the id is listed in the index.
func (idx *Index) Has(id backend.ID) bool {
	_, err := idx.Lookup(id)
	if err == nil {
		return true
	}

	return false
}

// LookupSize returns the length of the cleartext content behind the
// given id
func (idx *Index) LookupSize(id backend.ID) (cleartextLength uint, err error) {
	blob, err := idx.Lookup(id)
	if err != nil {
		return 0, err
	}
	return blob.PlaintextLength(), nil
}

// Merge loads all items from other into idx.
func (idx *Index) Merge(other *Index) {
	debug.Log("Index.Merge", "Merge index with %p", other)
	idx.m.Lock()
	defer idx.m.Unlock()

	for k, v := range other.pack {
		if _, ok := idx.pack[k]; ok {
			debug.Log("Index.Merge", "index already has key %v, updating", k.Str())
		}

		idx.pack[k] = v
	}
	debug.Log("Index.Merge", "done merging index")
}

// Supersedes returns the list of indexes this index supersedes, if any.
func (idx *Index) Supersedes() backend.IDs {
	return idx.supersedes
}

// AddToSupersedes adds the ids to the list of indexes superseded by this
// index. If the index has already been finalized, an error is returned.
func (idx *Index) AddToSupersedes(ids ...backend.ID) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	if idx.final {
		return errors.New("index already finalized")
	}

	idx.supersedes = append(idx.supersedes, ids...)
	return nil
}

// PackedBlob is a blob already saved within a pack.
type PackedBlob struct {
	Type   pack.BlobType
	Length uint
	ID     backend.ID
	Offset uint
	PackID backend.ID
}

func (pb PackedBlob) String() string {
	return fmt.Sprintf("<PackedBlob %v type %v in pack %v: len %v, offset %v",
		pb.ID.Str(), pb.Type, pb.PackID.Str(), pb.Length, pb.Offset)
}

// PlaintextLength returns the number of bytes the blob's plaintext occupies.
func (pb PackedBlob) PlaintextLength() uint {
	return pb.Length - crypto.Extension
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
				ID:     id,
				Offset: blob.offset,
				Type:   blob.tpe,
				Length: blob.length,
				PackID: blob.packID,
			}:
			}
		}
	}()

	return ch
}

// Packs returns all packs in this index
func (idx *Index) Packs() backend.IDSet {
	idx.m.Lock()
	defer idx.m.Unlock()

	packs := backend.NewIDSet()
	for _, entry := range idx.pack {
		packs.Insert(entry.packID)
	}

	return packs
}

// Count returns the number of blobs of type t in the index.
func (idx *Index) Count(t pack.BlobType) (n uint) {
	debug.Log("Index.Count", "counting blobs of type %v", t)
	idx.m.Lock()
	defer idx.m.Unlock()

	for id, blob := range idx.pack {
		if blob.tpe == t {
			n++
			debug.Log("Index.Count", "  blob %v counted: %v", id.Str(), blob)
		}
	}

	return
}

// Length returns the number of entries in the Index.
func (idx *Index) Length() uint {
	debug.Log("Index.Count", "counting blobs")
	idx.m.Lock()
	defer idx.m.Unlock()

	return uint(len(idx.pack))
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

// generatePackList returns a list of packs.
func (idx *Index) generatePackList() ([]*packJSON, error) {
	list := []*packJSON{}
	packs := make(map[backend.ID]*packJSON)

	for id, blob := range idx.pack {
		if blob.packID.IsNull() {
			panic("null pack id")
		}

		debug.Log("Index.generatePackList", "handle blob %v", id.Str())

		if blob.packID.IsNull() {
			debug.Log("Index.generatePackList", "blob %q has no packID! (type %v, offset %v, length %v)",
				id.Str(), blob.tpe, blob.offset, blob.length)
			return nil, fmt.Errorf("unable to serialize index: pack for blob %v hasn't been written yet", id)
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
			ID:     id,
			Type:   blob.tpe,
			Offset: blob.offset,
			Length: blob.length,
		})
	}

	debug.Log("Index.generatePackList", "done")

	return list, nil
}

type jsonIndex struct {
	Supersedes backend.IDs `json:"supersedes,omitempty"`
	Packs      []*packJSON `json:"packs"`
}

type jsonOldIndex []*packJSON

// Encode writes the JSON serialization of the index to the writer w.
func (idx *Index) Encode(w io.Writer) error {
	debug.Log("Index.Encode", "encoding index")
	idx.m.Lock()
	defer idx.m.Unlock()

	return idx.encode(w)
}

// encode writes the JSON serialization of the index to the writer w.
func (idx *Index) encode(w io.Writer) error {
	debug.Log("Index.encode", "encoding index")

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
	debug.Log("Index.Encode", "encoding index")
	idx.m.Lock()
	defer idx.m.Unlock()

	idx.final = true

	return idx.encode(w)
}

// ID returns the ID of the index, if available. If the index is not yet
// finalized, an error is returned.
func (idx *Index) ID() (backend.ID, error) {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return backend.ID{}, errors.New("index not finalized")
	}

	return idx.id, nil
}

// SetID sets the ID the index has been written to. This requires that
// Finalize() has been called before, otherwise an error is returned.
func (idx *Index) SetID(id backend.ID) error {
	idx.m.Lock()
	defer idx.m.Unlock()

	if !idx.final {
		return errors.New("indexs is not final")
	}

	if !idx.id.IsNull() {
		return errors.New("ID already set")
	}

	debug.Log("Index.SetID", "ID set to %v", id.Str())
	idx.id = id

	return nil
}

// Dump writes the pretty-printed JSON representation of the index to w.
func (idx *Index) Dump(w io.Writer) error {
	debug.Log("Index.Dump", "dumping index")
	idx.m.Lock()
	defer idx.m.Unlock()

	list, err := idx.generatePackList()
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
func DecodeIndex(rd io.Reader) (idx *Index, err error) {
	debug.Log("Index.DecodeIndex", "Start decoding index")
	idxJSON := jsonIndex{}

	dec := json.NewDecoder(rd)
	err = dec.Decode(&idxJSON)
	if err != nil {
		debug.Log("Index.DecodeIndex", "Error %v", err)

		if isErrOldIndex(err) {
			debug.Log("Index.DecodeIndex", "index is probably old format, trying that")
			err = ErrOldIndexFormat
		}

		return nil, err
	}

	idx = NewIndex()
	for _, pack := range idxJSON.Packs {
		for _, blob := range pack.Blobs {
			idx.store(blob.Type, blob.ID, pack.ID, blob.Offset, blob.Length)
		}
	}
	idx.supersedes = idxJSON.Supersedes
	idx.final = true

	debug.Log("Index.DecodeIndex", "done")
	return idx, err
}

// DecodeOldIndex loads and unserializes an index in the old format from rd.
func DecodeOldIndex(rd io.Reader) (idx *Index, err error) {
	debug.Log("Index.DecodeOldIndex", "Start decoding old index")
	list := []*packJSON{}

	dec := json.NewDecoder(rd)
	err = dec.Decode(&list)
	if err != nil {
		debug.Log("Index.DecodeOldIndex", "Error %#v", err)
		return nil, err
	}

	idx = NewIndex()
	for _, pack := range list {
		for _, blob := range pack.Blobs {
			idx.store(blob.Type, blob.ID, pack.ID, blob.Offset, blob.Length)
		}
	}

	debug.Log("Index.DecodeOldIndex", "done")
	return idx, err
}

// ConvertIndexes loads all indexes from the repo and converts them to the new
// format (if necessary). When the conversion is succcessful, the old indexes
// are removed.
func ConvertIndexes(repo *Repository) error {
	debug.Log("ConvertIndexes", "start")
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(backend.Index, done) {
		debug.Log("ConvertIndexes", "checking index %v", id.Str())

		newID, err := ConvertIndex(repo, id)
		if err != nil {
			debug.Log("ConvertIndexes", "Converting index %v returns error: %v", id.Str(), err)
			return err
		}

		if id != newID {
			debug.Log("ConvertIndexes", "index %v converted to new format as %v", id.Str(), newID.Str())
		}
	}

	debug.Log("ConvertIndexes", "done")
	return nil
}

// LoadIndexWithDecoder loads the index and decodes it with fn.
func LoadIndexWithDecoder(repo *Repository, id string, fn func(io.Reader) (*Index, error)) (*Index, error) {
	debug.Log("LoadIndexWithDecoder", "Loading index %v", id[:8])

	idxID, err := backend.ParseID(id)
	if err != nil {
		return nil, err
	}

	rd, err := repo.GetDecryptReader(backend.Index, idxID.String())
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	idx, err := fn(rd)
	if err != nil {
		debug.Log("LoadIndexWithDecoder", "error while decoding index %v: %v", id, err)
		return nil, err
	}

	idx.id = idxID

	return idx, nil
}

// ConvertIndex loads the given index from the repo and converts them to the new
// format (if necessary). When the conversion is succcessful, the old index
// is removed. Returned is either the old id (if no conversion was needed) or
// the new id.
func ConvertIndex(repo *Repository, id backend.ID) (backend.ID, error) {
	debug.Log("ConvertIndex", "checking index %v", id.Str())

	idx, err := LoadIndexWithDecoder(repo, id.String(), DecodeOldIndex)
	if err != nil {
		debug.Log("ConvertIndex", "LoadIndexWithDecoder(%v) returned error: %v", id.Str(), err)
		return id, err
	}

	blob, err := repo.CreateEncryptedBlob(backend.Index)
	if err != nil {
		return id, err
	}

	idx.supersedes = backend.IDs{id}

	err = idx.Encode(blob)
	if err != nil {
		debug.Log("ConvertIndex", "oldIdx.Encode() returned error: %v", err)
		return id, err
	}

	err = blob.Close()
	if err != nil {
		debug.Log("ConvertIndex", "blob.Close() returned error: %v", err)
		return id, err
	}

	newID := blob.ID()
	debug.Log("ConvertIndex", "index %v converted to new format as %v", id.Str(), newID.Str())

	err = repo.be.Remove(backend.Index, id.String())
	if err != nil {
		debug.Log("ConvertIndex", "backend.Remove(%v) returned error: %v", id.Str(), err)
		return id, err
	}

	return newID, nil
}
