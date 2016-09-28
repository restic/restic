// Package index contains various data structures for indexing content in a repository or backend.
package index

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"restic/list"
	"restic/worker"

	"restic/errors"
)

// Pack contains information about the contents of a pack.
type Pack struct {
	Size    int64
	Entries []restic.Blob
}

// Blob contains information about a blob.
type Blob struct {
	Size  int64
	Packs restic.IDSet
}

// Index contains information about blobs and packs stored in a repo.
type Index struct {
	Packs    map[restic.ID]Pack
	Blobs    map[restic.BlobHandle]Blob
	IndexIDs restic.IDSet
}

func newIndex() *Index {
	return &Index{
		Packs:    make(map[restic.ID]Pack),
		Blobs:    make(map[restic.BlobHandle]Blob),
		IndexIDs: restic.NewIDSet(),
	}
}

// New creates a new index for repo from scratch.
func New(repo restic.Repository, p *restic.Progress) (*Index, error) {
	done := make(chan struct{})
	defer close(done)

	p.Start()
	defer p.Done()

	ch := make(chan worker.Job)
	go list.AllPacks(repo, ch, done)

	idx := newIndex()

	for job := range ch {
		p.Report(restic.Stat{Blobs: 1})

		packID := job.Data.(restic.ID)
		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "unable to list pack %v: %v\n", packID.Str(), job.Error)
			continue
		}

		j := job.Result.(list.Result)

		debug.Log("pack %v contains %d blobs", packID.Str(), len(j.Entries()))

		err := idx.AddPack(packID, j.Size(), j.Entries())
		if err != nil {
			return nil, err
		}

		p := Pack{Entries: j.Entries(), Size: j.Size()}
		idx.Packs[packID] = p
	}

	return idx, nil
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

type indexJSON struct {
	Supersedes restic.IDs  `json:"supersedes,omitempty"`
	Packs      []*packJSON `json:"packs"`
}

func loadIndexJSON(repo restic.Repository, id restic.ID) (*indexJSON, error) {
	debug.Log("process index %v\n", id.Str())

	var idx indexJSON
	err := repo.LoadJSONUnpacked(restic.IndexFile, id, &idx)
	if err != nil {
		return nil, err
	}

	return &idx, nil
}

// Load creates an index by loading all index files from the repo.
func Load(repo restic.Repository, p *restic.Progress) (*Index, error) {
	debug.Log("loading indexes")

	p.Start()
	defer p.Done()

	done := make(chan struct{})
	defer close(done)

	supersedes := make(map[restic.ID]restic.IDSet)
	results := make(map[restic.ID]map[restic.ID]Pack)

	index := newIndex()

	for id := range repo.List(restic.IndexFile, done) {
		p.Report(restic.Stat{Blobs: 1})

		debug.Log("Load index %v", id.Str())
		idx, err := loadIndexJSON(repo, id)
		if err != nil {
			return nil, err
		}

		res := make(map[restic.ID]Pack)
		supersedes[id] = restic.NewIDSet()
		for _, sid := range idx.Supersedes {
			debug.Log("  index %v supersedes %v", id.Str(), sid)
			supersedes[id].Insert(sid)
		}

		for _, jpack := range idx.Packs {
			entries := make([]restic.Blob, 0, len(jpack.Blobs))
			for _, blob := range jpack.Blobs {
				entry := restic.Blob{
					ID:     blob.ID,
					Type:   blob.Type,
					Offset: blob.Offset,
					Length: blob.Length,
				}
				entries = append(entries, entry)
			}

			if err = index.AddPack(jpack.ID, 0, entries); err != nil {
				return nil, err
			}
		}

		results[id] = res
		index.IndexIDs.Insert(id)
	}

	for superID, list := range supersedes {
		for indexID := range list {
			if _, ok := results[indexID]; !ok {
				continue
			}
			debug.Log("  removing index %v, superseded by %v", indexID.Str(), superID.Str())
			fmt.Fprintf(os.Stderr, "index %v can be removed, superseded by index %v\n", indexID.Str(), superID.Str())
			delete(results, indexID)
		}
	}

	return index, nil
}

// AddPack adds a pack to the index. If this pack is already in the index, an
// error is returned.
func (idx *Index) AddPack(id restic.ID, size int64, entries []restic.Blob) error {
	if _, ok := idx.Packs[id]; ok {
		return errors.Errorf("pack %v already present in the index", id.Str())
	}

	idx.Packs[id] = Pack{Size: size, Entries: entries}

	for _, entry := range entries {
		h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
		if _, ok := idx.Blobs[h]; !ok {
			idx.Blobs[h] = Blob{
				Size:  int64(entry.Length),
				Packs: restic.NewIDSet(),
			}
		}

		idx.Blobs[h].Packs.Insert(id)
	}

	return nil
}

// RemovePack deletes a pack from the index.
func (idx *Index) RemovePack(id restic.ID) error {
	if _, ok := idx.Packs[id]; !ok {
		return errors.Errorf("pack %v not found in the index", id.Str())
	}

	for _, blob := range idx.Packs[id].Entries {
		h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
		idx.Blobs[h].Packs.Delete(id)

		if len(idx.Blobs[h].Packs) == 0 {
			delete(idx.Blobs, h)
		}
	}

	delete(idx.Packs, id)

	return nil
}

// DuplicateBlobs returns a list of blobs that are stored more than once in the
// repo.
func (idx *Index) DuplicateBlobs() (dups restic.BlobSet) {
	dups = restic.NewBlobSet()
	seen := restic.NewBlobSet()

	for _, p := range idx.Packs {
		for _, entry := range p.Entries {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if seen.Has(h) {
				dups.Insert(h)
			}
			seen.Insert(h)
		}
	}

	return dups
}

// PacksForBlobs returns the set of packs in which the blobs are contained.
func (idx *Index) PacksForBlobs(blobs restic.BlobSet) (packs restic.IDSet) {
	packs = restic.NewIDSet()

	for h := range blobs {
		blob, ok := idx.Blobs[h]
		if !ok {
			continue
		}

		for id := range blob.Packs {
			packs.Insert(id)
		}
	}

	return packs
}

// Location describes the location of a blob in a pack.
type Location struct {
	PackID restic.ID
	restic.Blob
}

// ErrBlobNotFound is return by FindBlob when the blob could not be found in
// the index.
var ErrBlobNotFound = errors.New("blob not found in index")

// FindBlob returns a list of packs and positions the blob can be found in.
func (idx *Index) FindBlob(h restic.BlobHandle) ([]Location, error) {
	blob, ok := idx.Blobs[h]
	if !ok {
		return nil, ErrBlobNotFound
	}

	result := make([]Location, 0, len(blob.Packs))
	for packID := range blob.Packs {
		pack, ok := idx.Packs[packID]
		if !ok {
			return nil, errors.Errorf("pack %v not found in index", packID.Str())
		}

		for _, entry := range pack.Entries {
			if entry.Type != h.Type {
				continue
			}

			if !entry.ID.Equal(h.ID) {
				continue
			}

			loc := Location{PackID: packID, Blob: entry}
			result = append(result, loc)
		}
	}

	return result, nil
}

// Save writes the complete index to the repo.
func (idx *Index) Save(repo restic.Repository, supersedes restic.IDs) (restic.ID, error) {
	packs := make(map[restic.ID][]restic.Blob, len(idx.Packs))
	for id, p := range idx.Packs {
		packs[id] = p.Entries
	}

	return Save(repo, packs, supersedes)
}

// Save writes a new index containing the given packs.
func Save(repo restic.Repository, packs map[restic.ID][]restic.Blob, supersedes restic.IDs) (restic.ID, error) {
	idx := &indexJSON{
		Supersedes: supersedes,
		Packs:      make([]*packJSON, 0, len(packs)),
	}

	for packID, blobs := range packs {
		b := make([]blobJSON, 0, len(blobs))
		for _, blob := range blobs {
			b = append(b, blobJSON{
				ID:     blob.ID,
				Type:   blob.Type,
				Offset: blob.Offset,
				Length: blob.Length,
			})
		}

		p := &packJSON{
			ID:    packID,
			Blobs: b,
		}

		idx.Packs = append(idx.Packs, p)
	}

	return repo.SaveJSONUnpacked(restic.IndexFile, idx)
}
