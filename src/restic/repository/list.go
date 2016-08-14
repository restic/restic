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
	packID  backend.ID
	size    int64
	entries []pack.Blob
}

// PackID returns the pack ID of this result.
func (l ListAllPacksResult) PackID() backend.ID {
	return l.packID
}

// Size ruturns the size of the pack.
func (l ListAllPacksResult) Size() int64 {
	return l.size
}

// Entries returns a list of all blobs saved in the pack.
func (l ListAllPacksResult) Entries() []pack.Blob {
	return l.entries
}

// ListAllPacks sends the contents of all packs to ch.
func ListAllPacks(repo Lister, ch chan<- worker.Job, done <-chan struct{}) {
	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		packID := job.Data.(backend.ID)
		entries, size, err := repo.ListPack(packID)

		return ListAllPacksResult{
			packID:  packID,
			size:    size,
			entries: entries,
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
