package repository

import (
	"restic/backend"
	"restic/pack"
	"restic/worker"
)

const listPackWorkers = 10

// Lister combines lists packs in a repo and blobs in a pack.
type Lister interface {
	List(backend.Type, <-chan struct{}) <-chan backend.ID
	ListPack(backend.ID) ([]pack.Blob, int64, error)
}

// ListAllPacksResult is returned in the channel from LoadBlobsFromAllPacks.
type ListAllPacksResult struct {
	PackID  backend.ID
	Size    int64
	Entries []pack.Blob
}

// ListAllPacks sends the contents of all packs to ch.
func ListAllPacks(repo Lister, ch chan<- worker.Job, done <-chan struct{}) {
	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		packID := job.Data.(backend.ID)
		entries, size, err := repo.ListPack(packID)

		return ListAllPacksResult{
			PackID:  packID,
			Size:    size,
			Entries: entries,
		}, err
	}

	jobCh := make(chan worker.Job)
	wp := worker.New(listPackWorkers, f, jobCh, ch)

	go func() {
		defer close(jobCh)
		for id := range repo.List(backend.Data, done) {
			select {
			case jobCh <- worker.Job{Data: id}:
			case <-done:
				return
			}
		}
	}()

	wp.Wait()
}
