package repository

import (
	"fmt"
	"os"
	"restic/backend"
	"restic/debug"
	"restic/list"
	"restic/worker"
)

// RebuildIndex lists all packs in the repo, writes a new index and removes all
// old indexes. This operation should only be done with an exclusive lock in
// place.
func RebuildIndex(repo *Repository) error {
	debug.Log("RebuildIndex", "start rebuilding index")

	done := make(chan struct{})
	defer close(done)

	ch := make(chan worker.Job)
	go list.AllPacks(repo, ch, done)

	idx := NewIndex()
	for job := range ch {
		id := job.Data.(backend.ID)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", id, job.Error)
			continue
		}

		res := job.Result.(list.Result)

		for _, entry := range res.Entries() {
			pb := PackedBlob{
				ID:     entry.ID,
				Type:   entry.Type,
				Length: entry.Length,
				Offset: entry.Offset,
				PackID: res.PackID(),
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
