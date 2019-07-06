// Package index contains various data structures for indexing content in a repository or backend.
package index

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
	tomb "gopkg.in/tomb.v2"
)

// Pack contains information about the contents of a pack.
type Pack struct {
	ID      restic.ID
	Size    int64
	Entries []restic.Blob
}

// Index contains information about blobs and packs stored in a repo.
type Index struct {
	m        sync.Mutex
	Packs    map[restic.ID]Pack
	IndexIDs restic.IDSet
}

func newIndex() *Index {
	return &Index{
		Packs:    make(map[restic.ID]Pack),
		IndexIDs: restic.NewIDSet(),
	}
}

// Lister lists files and their contents
type Lister interface {
	// List runs fn for all files of type t in the repo.
	List(ctx context.Context, t restic.FileType, fn func(restic.ID, int64) error) error

	// ListPack returns the list of blobs saved in the pack id and the length
	// of the file as stored in the backend.
	ListPack(ctx context.Context, id restic.ID, size int64) ([]restic.Blob, int64, error)
}

// New creates a new index for repo from scratch. InvalidFiles contains all IDs
// of files  that cannot be listed successfully.
func New(ctx context.Context, repo restic.Repository, ignorePacks restic.IDSet, p *restic.Progress) (idx *Index, invalidFiles restic.IDs, err error) {
	p.Start()
	defer p.Done()

	type Job struct {
		PackID restic.ID
		Size   int64
	}

	type Result struct {
		Error   error
		PackID  restic.ID
		Size    int64
		Entries []restic.Blob
	}

	inputCh := make(chan Job)
	outputCh := make(chan Result)
	wg, ctx := errgroup.WithContext(ctx)

	// list the files in the repo, send to inputCh
	wg.Go(func() error {
		defer close(inputCh)
		return repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
			if ignorePacks.Has(id) {
				return nil
			}

			job := Job{
				PackID: id,
				Size:   size,
			}

			select {
			case inputCh <- job:
			case <-ctx.Done():
			}
			return nil
		})
	})

	// run the workers listing the files, read from inputCh, send to outputCh
	var workers sync.WaitGroup
	for i := uint(0); i < repo.Backend().Connections(); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range inputCh {
				res := Result{PackID: job.PackID}
				res.Entries, res.Size, res.Error = repo.ListPack(ctx, job.PackID, job.Size)

				select {
				case outputCh <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// wait until all the workers are done, then close outputCh
	wg.Go(func() error {
		workers.Wait()
		close(outputCh)
		return nil
	})

	idx = newIndex()

	for res := range outputCh {
		p.Report(restic.Stat{Blobs: 1})
		if res.Error != nil {
			cause := errors.Cause(res.Error)
			if _, ok := cause.(pack.InvalidFileError); ok {
				invalidFiles = append(invalidFiles, res.PackID)
				continue
			}

			fmt.Fprintf(os.Stderr, "pack file cannot be listed %v: %v\n", res.PackID, res.Error)
			continue
		}

		debug.Log("pack %v contains %d blobs", res.PackID, len(res.Entries))

		err := idx.AddPack(res.PackID, res.Size, res.Entries)
		if err != nil {
			return nil, nil, err
		}

		select {
		case <-ctx.Done(): // an error occurred
		default:
		}
	}

	err = wg.Wait()
	if err != nil {
		return nil, nil, err
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
	Supersedes restic.IDs `json:"supersedes,omitempty"`
	Packs      []packJSON `json:"packs"`
}

// ListLoader allows listing files and their content, in addition to loading and unmarshaling JSON files.
type ListLoader interface {
	Lister
	LoadJSONUnpacked(context.Context, restic.FileType, restic.ID, interface{}) error
}

func loadIndexJSON(ctx context.Context, repo ListLoader, id restic.ID) (*indexJSON, error) {
	debug.Log("process index %v\n", id)

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

	t, ctx := tomb.WithContext(ctx)

	type ReadJob struct {
		IndexID restic.ID
		Size    int64
	}
	type SaveJob struct {
		IndexID    restic.ID
		Packs      map[restic.ID]Pack
		Supersedes restic.IDSet
	}

	readCh := make(chan ReadJob)
	saveCh := make(chan SaveJob)

	supersedes := make(map[restic.ID]restic.IDSet)
	results := make(map[restic.ID]map[restic.ID]Pack)

	index := newIndex()

	// List index files in the repo, send to readCh
	listWorker := func() error {
		defer close(readCh)
		err := repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
			select {
			case readCh <- ReadJob{IndexID: id, Size: size}:
			case <-ctx.Done():
				return tomb.ErrDying
			}
			return nil
		})
		return err
	}

	// Read index files received on readCh, parse to partial indexes, send
	// to saveCh
	var readWg sync.WaitGroup
	readWorker := func() error {
		defer readWg.Done()
		for job := range readCh {
			debug.Log("Load index %v", job.IndexID)

			idx, err := loadIndexJSON(ctx, repo, job.IndexID)
			if err != nil {
				return err
			}

			saveJob := SaveJob{
				IndexID:    job.IndexID,
				Packs:      make(map[restic.ID]Pack),
				Supersedes: restic.NewIDSet(),
			}

			for _, sid := range idx.Supersedes {
				debug.Log("  index %v supersedes %v", job.IndexID, sid)
				saveJob.Supersedes.Insert(sid)
			}

			for _, pack := range idx.Packs {
				entries := make([]restic.Blob, 0, len(pack.Blobs))
				for _, blob := range pack.Blobs {
					entry := restic.Blob{
						ID:     blob.ID,
						Type:   blob.Type,
						Offset: blob.Offset,
						Length: blob.Length,
					}
					entries = append(entries, entry)
				}
				saveJob.Packs[pack.ID] = Pack{
					ID:      pack.ID,
					Size:    job.Size,
					Entries: entries,
				}
			}

			select {
			case saveCh <- saveJob:
			case <-ctx.Done():
				return tomb.ErrDying
			}

			p.Report(restic.Stat{Blobs: 1})
		}
		return nil
	}

	// Collect partial indexes from saveCh
	saveWorker := func() error {
		for job := range saveCh {
			supersedes[job.IndexID] = job.Supersedes
			results[job.IndexID] = job.Packs
			index.IndexIDs.Insert(job.IndexID)
		}
		return nil
	}

	t.Go(func() error {
		t.Go(listWorker)

		readWorkers := repo.Backend().Connections()
		readWg.Add(int(readWorkers))
		for i := uint(0); i < readWorkers; i++ {
			t.Go(readWorker)
		}
		t.Go(func() error {
			readWg.Wait()
			close(saveCh)
			return nil
		})

		t.Go(saveWorker)

		return nil
	})

	if err := t.Wait(); err != nil {
		return nil, err
	}

	// Ignore superseded index files
	for superID, list := range supersedes {
		for indexID := range list {
			if _, ok := results[indexID]; !ok {
				continue
			}
			debug.Log("  removing index %v, superseded by %v", indexID, superID)
			fmt.Fprintf(os.Stderr, "index %v can be removed, superseded by index %v\n", indexID.Str(), superID.Str())
			delete(results, indexID)
		}
	}

	// Combine the partial indexes
	for _, idx := range results {
		for _, pack := range idx {
			if _, ok := index.Packs[pack.ID]; ok {
				continue
			}
			err := index.AddPack(pack.ID, 0, pack.Entries)
			if err != nil {
				return nil, err
			}
		}
	}

	return index, nil
}

// AddPack adds a pack to the index. If this pack is already in the index, an
// error is returned.
func (idx *Index) AddPack(id restic.ID, size int64, entries []restic.Blob) error {
	idx.m.Lock()
	defer idx.m.Unlock()

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

const maxEntries = 3000

// Saver saves structures as JSON.
type Saver interface {
	SaveJSONUnpacked(ctx context.Context, t restic.FileType, item interface{}) (restic.ID, error)
}

// Save writes the complete index to the repo.
func (idx *Index) Save(ctx context.Context, repo Saver, supersedes restic.IDs, prog *restic.Progress) (restic.IDs, error) {
	prog.Start()
	defer prog.Done()

	debug.Log("pack files: %d\n", len(idx.Packs))

	var indexIDs []restic.ID

	packs := 0
	jsonIDX := &indexJSON{
		Supersedes: supersedes,
		Packs:      make([]packJSON, 0, maxEntries),
	}

	for packID, pack := range idx.Packs {
		debug.Log("%04d add pack %v with %d entries", packs, packID, len(pack.Entries))
		b := make([]blobJSON, 0, len(pack.Entries))
		for _, blob := range pack.Entries {
			b = append(b, blobJSON{
				ID:     blob.ID,
				Type:   blob.Type,
				Offset: blob.Offset,
				Length: blob.Length,
			})
		}

		p := packJSON{
			ID:    packID,
			Blobs: b,
		}

		jsonIDX.Packs = append(jsonIDX.Packs, p)

		packs++
		if packs == maxEntries {
			id, err := repo.SaveJSONUnpacked(ctx, restic.IndexFile, jsonIDX)
			if err != nil {
				return nil, err
			}
			debug.Log("saved new index as %v", id)
			prog.Report(restic.Stat{Blobs: 1})

			indexIDs = append(indexIDs, id)
			packs = 0
			jsonIDX.Packs = jsonIDX.Packs[:0]
		}
	}

	if packs > 0 {
		id, err := repo.SaveJSONUnpacked(ctx, restic.IndexFile, jsonIDX)
		if err != nil {
			return nil, err
		}
		debug.Log("saved new index as %v", id)
		prog.Report(restic.Stat{Blobs: 1})
		indexIDs = append(indexIDs, id)
	}

	return indexIDs, nil
}
