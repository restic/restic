package repository

import (
	"fmt"
	"os"
	"restic/backend"
	"restic/debug"
	"restic/pack"
	"restic/worker"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed.
func Repack(repo *Repository, packs, keepBlobs backend.IDSet) error {
	debug.Log("Repack", "repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	var buf []byte
	for packID := range packs {
		list, err := repo.ListPack(packID)
		if err != nil {
			return err
		}

		debug.Log("Repack", "processing pack %v, blobs: %v", packID.Str(), list)

		for _, blob := range list {
			buf, err = repo.LoadBlob(blob.Type, blob.ID, buf)
			if err != nil {
				return err
			}
			debug.Log("Repack", "  loaded blob %v", blob.ID.Str())

			_, err = repo.SaveAndEncrypt(blob.Type, buf, &blob.ID)
			if err != nil {
				return err
			}

			debug.Log("Repack", "  saved blob %v", blob.ID.Str())
		}
	}

	if err := repo.Flush(); err != nil {
		return err
	}

	for packID := range packs {
		err := repo.Backend().Remove(backend.Data, packID.String())
		if err != nil {
			debug.Log("Repack", "error removing pack %v: %v", packID.Str(), err)
			return err
		}
		debug.Log("Repack", "removed pack %v", packID.Str())
	}

	return nil
}

const rebuildIndexWorkers = 10

type loadBlobsResult struct {
	packID  backend.ID
	entries []pack.Blob
}

// loadBlobsFromAllPacks sends the contents of all packs to ch.
func loadBlobsFromAllPacks(repo *Repository, ch chan<- worker.Job, done <-chan struct{}) {
	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		packID := job.Data.(backend.ID)
		entries, err := repo.ListPack(packID)
		return loadBlobsResult{
			packID:  packID,
			entries: entries,
		}, err
	}

	jobCh := make(chan worker.Job)
	wp := worker.New(rebuildIndexWorkers, f, jobCh, ch)

	go func() {
		for id := range repo.List(backend.Data, done) {
			jobCh <- worker.Job{Data: id}
		}
		close(jobCh)
	}()

	wp.Wait()
}

// RebuildIndex lists all packs in the repo, writes a new index and removes all
// old indexes. This operation should only be done with an exclusive lock in
// place.
func RebuildIndex(repo *Repository) error {
	debug.Log("RebuildIndex", "start rebuilding index")

	done := make(chan struct{})
	defer close(done)

	ch := make(chan worker.Job)
	go loadBlobsFromAllPacks(repo, ch, done)

	idx := NewIndex()
	for job := range ch {
		id := job.Data.(backend.ID)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", id, job.Error)
			continue
		}

		res := job.Result.(loadBlobsResult)

		for _, entry := range res.entries {
			pb := PackedBlob{
				ID:     entry.ID,
				Type:   entry.Type,
				Length: entry.Length,
				Offset: entry.Offset,
				PackID: res.packID,
			}
			idx.Store(pb)
		}
	}

	oldIndexes := backend.NewIDSet()
	for id := range repo.List(backend.Index, done) {
		idx.AddToSupersedes(id)
		oldIndexes.Insert(id)
	}

	id, err := SaveIndex(repo, idx)
	if err != nil {
		debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
		return err
	}
	debug.Log("RebuildIndex.RebuildIndex", "new index saved as %v", id.Str())

	for indexID := range oldIndexes {
		err := repo.Backend().Remove(backend.Index, indexID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove index %v: %v\n", indexID.Str(), err)
		}
	}

	return nil
}
