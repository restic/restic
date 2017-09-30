// Package index contains various data structures for indexing content in a repository or backend.
package index

import (
	"context"
	"fmt"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/list"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/worker"

	"github.com/restic/restic/internal/errors"
)

// Pack contains information about the contents of a pack.
type Pack struct {
	ID      restic.ID
	Size    int64
	Entries []restic.Blob
}

// Index contains information about blobs and packs stored in a repo.
type Index struct {
	Packs    map[restic.ID]Pack
	IndexIDs restic.IDSet
}

func newIndex() *Index {
	return &Index{
		Packs:    make(map[restic.ID]Pack),
		IndexIDs: restic.NewIDSet(),
	}
}

// New creates a new index for repo from scratch. InvalidFiles contains all IDs
// of files  that cannot be listed successfully.
func New(ctx context.Context, repo restic.Repository, ignorePacks restic.IDSet, p *restic.Progress) (idx *Index, invalidFiles restic.IDs, err error) {
	p.Start()
	defer p.Done()

	ch := make(chan worker.Job)
	go list.AllPacks(ctx, repo, ignorePacks, ch)

	idx = newIndex()

	for job := range ch {
		p.Report(restic.Stat{Blobs: 1})

		packID := job.Data.(restic.ID)
		if job.Error != nil {
			cause := errors.Cause(job.Error)
			if _, ok := cause.(pack.InvalidFileError); ok {
				invalidFiles = append(invalidFiles, packID)
				continue
			}

			fmt.Fprintf(os.Stderr, "pack file cannot be listed %v: %v\n", packID.Str(), job.Error)
			continue
		}

		j := job.Result.(list.Result)

		debug.Log("pack %v contains %d blobs", packID.Str(), len(j.Entries()))

		err := idx.AddPack(packID, j.Size(), j.Entries())
		if err != nil {
			return nil, nil, err
		}
	}

	return idx, invalidFiles, nil
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

func loadIndexJSON(ctx context.Context, repo restic.Repository, id restic.ID) (*indexJSON, error) {
	debug.Log("process index %v\n", id.Str())

	var idx indexJSON
	err := repo.LoadJSONUnpacked(ctx, restic.IndexFile, id, &idx)
	if err != nil {
		return nil, err
	}

	return &idx, nil
}

// Load creates an index by loading all index files from the repo.
func Load(ctx context.Context, repo restic.Repository, p *restic.Progress) (*Index, error) {
	debug.Log("loading indexes")

	p.Start()
	defer p.Done()

	supersedes := make(map[restic.ID]restic.IDSet)
	results := make(map[restic.ID]map[restic.ID]Pack)

	index := newIndex()

	for id := range repo.List(ctx, restic.IndexFile) {
		p.Report(restic.Stat{Blobs: 1})

		debug.Log("Load index %v", id.Str())
		idx, err := loadIndexJSON(ctx, repo, id)
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

	idx.Packs[id] = Pack{ID: id, Size: size, Entries: entries}

	return nil
}

// RemovePack deletes a pack from the index.
func (idx *Index) RemovePack(id restic.ID) error {
	if _, ok := idx.Packs[id]; !ok {
		return errors.Errorf("pack %v not found in the index", id.Str())
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

	for id, p := range idx.Packs {
		for _, entry := range p.Entries {
			if blobs.Has(restic.BlobHandle{ID: entry.ID, Type: entry.Type}) {
				packs.Insert(id)
			}
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
func (idx *Index) FindBlob(h restic.BlobHandle) (result []Location, err error) {
	for id, p := range idx.Packs {
		for _, entry := range p.Entries {
			if entry.ID.Equal(h.ID) && entry.Type == h.Type {
				result = append(result, Location{
					PackID: id,
					Blob:   entry,
				})
			}
		}
	}

	if len(result) == 0 {
		return nil, ErrBlobNotFound
	}

	return result, nil
}

// Save writes the complete index to the repo.
func (idx *Index) Save(ctx context.Context, repo restic.Repository, supersedes restic.IDs) (restic.ID, uint64, error) {
	packs := make(map[restic.ID][]restic.Blob, len(idx.Packs))
	for id, p := range idx.Packs {
		packs[id] = p.Entries
	}

	return Save(ctx, repo, packs, supersedes)
}

// Save writes a new index containing the given packs.
func Save(ctx context.Context, repo restic.Repository, packs map[restic.ID][]restic.Blob, supersedes restic.IDs) (restic.ID, uint64, error) {
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

	return repo.SaveJSONUnpacked(ctx, restic.IndexFile, idx)
}
