// Package index contains various data structures for indexing content in a repository or backend.
package index

import (
	"fmt"
	"os"
	"restic/backend"
	"restic/debug"
	"restic/pack"
	"restic/repository"
	"restic/worker"
)

// Pack contains information about the contents of a pack.
type Pack struct {
	Entries []pack.Blob
}

// Index contains information about blobs and packs stored in a repo.
type Index struct {
	Packs map[backend.ID]Pack
}

func newIndex() *Index {
	return &Index{
		Packs: make(map[backend.ID]Pack),
	}
}

// New creates a new index for repo from scratch.
func New(repo *repository.Repository) (*Index, error) {
	done := make(chan struct{})
	defer close(done)

	ch := make(chan worker.Job)
	go repository.ListAllPacks(repo, ch, done)

	idx := newIndex()

	for job := range ch {
		packID := job.Data.(backend.ID)
		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "unable to list pack %v: %v\n", packID.Str(), job.Error)
			continue
		}

		j := job.Result.(repository.ListAllPacksResult)

		debug.Log("Index.New", "pack %v contains %d blobs", packID.Str(), len(j.Entries))

		if _, ok := idx.Packs[packID]; ok {
			return nil, fmt.Errorf("pack %v processed twice", packID.Str())
		}
		p := Pack{Entries: j.Entries}
		idx.Packs[packID] = p
	}

	return idx, nil
}

const loadIndexParallelism = 20

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

type indexJSON struct {
	Supersedes backend.IDs `json:"supersedes,omitempty"`
	Packs      []*packJSON `json:"packs"`
}

func loadIndexJSON(repo *repository.Repository, id backend.ID) (*indexJSON, error) {
	debug.Log("index.loadIndexJSON", "process index %v\n", id.Str())

	var idx indexJSON
	err := repo.LoadJSONUnpacked(backend.Index, id, &idx)
	if err != nil {
		return nil, err
	}

	return &idx, nil
}

// Load creates an index by loading all index files from the repo.
func Load(repo *repository.Repository) (*Index, error) {
	debug.Log("index.Load", "loading indexes")

	done := make(chan struct{})
	defer close(done)

	supersedes := make(map[backend.ID]backend.IDSet)
	results := make(map[backend.ID]map[backend.ID]Pack)

	for id := range repo.List(backend.Index, done) {
		debug.Log("index.Load", "Load index %v", id.Str())
		idx, err := loadIndexJSON(repo, id)
		if err != nil {
			return nil, err
		}

		res := make(map[backend.ID]Pack)
		supersedes[id] = backend.NewIDSet()
		for _, sid := range idx.Supersedes {
			debug.Log("index.Load", "  index %v supersedes %v", id.Str(), sid)
			supersedes[id].Insert(sid)
		}

		for _, jpack := range idx.Packs {
			P := Pack{}
			for _, blob := range jpack.Blobs {
				entry := pack.Blob{
					ID:     blob.ID,
					Type:   blob.Type,
					Offset: blob.Offset,
					Length: blob.Length,
				}
				P.Entries = append(P.Entries, entry)
			}
			res[jpack.ID] = P
		}

		results[id] = res
	}

	for superID, list := range supersedes {
		for indexID := range list {
			debug.Log("index.Load", "  removing index %v, superseded by %v", indexID.Str(), superID.Str())
			delete(results, indexID)
		}
	}

	idx := newIndex()
	for _, packs := range results {
		for id, pack := range packs {
			idx.Packs[id] = pack
		}
	}

	return idx, nil
}
