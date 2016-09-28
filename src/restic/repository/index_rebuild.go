package repository

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"restic/list"
	"restic/worker"
)

// RebuildIndex lists all packs in the repo, writes a new index and removes all
// old indexes. This operation should only be done with an exclusive lock in
// place.
func RebuildIndex(repo restic.Repository) error {
	debug.Log("start rebuilding index")

	done := make(chan struct{})
	defer close(done)

	ch := make(chan worker.Job)
	go list.AllPacks(repo, ch, done)

	idx := NewIndex()
	for job := range ch {
		id := job.Data.(restic.ID)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", id, job.Error)
			continue
		}

		res := job.Result.(list.Result)

		for _, entry := range res.Entries() {
			pb := restic.PackedBlob{
				Blob:   entry,
				PackID: res.PackID(),
			}
			idx.Store(pb)
		}
	}

	oldIndexes := restic.NewIDSet()
	for id := range repo.List(restic.IndexFile, done) {
		idx.AddToSupersedes(id)
		oldIndexes.Insert(id)
	}

	id, err := SaveIndex(repo, idx)
	if err != nil {
		debug.Log("error saving index: %v", err)
		return err
	}
	debug.Log("new index saved as %v", id.Str())

	for indexID := range oldIndexes {
		err := repo.Backend().Remove(restic.IndexFile, indexID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove index %v: %v\n", indexID.Str(), err)
		}
	}

	return nil
}
