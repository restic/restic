package list

import (
	"restic"
	"restic/worker"
)

const listPackWorkers = 10

// Lister combines lists packs in a repo and blobs in a pack.
type Lister interface {
	List(restic.FileType, <-chan struct{}) <-chan restic.ID
	ListPack(restic.ID) ([]restic.Blob, int64, error)
}

// Result is returned in the channel from LoadBlobsFromAllPacks.
type Result struct {
	packID  restic.ID
	size    int64
	entries []restic.Blob
}

// PackID returns the pack ID of this result.
func (l Result) PackID() restic.ID {
	return l.packID
}

// Size ruturns the size of the pack.
func (l Result) Size() int64 {
	return l.size
}

// Entries returns a list of all blobs saved in the pack.
func (l Result) Entries() []restic.Blob {
	return l.entries
}

// AllPacks sends the contents of all packs to ch.
func AllPacks(repo Lister, ch chan<- worker.Job, done <-chan struct{}) {
	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		packID := job.Data.(restic.ID)
		entries, size, err := repo.ListPack(packID)

		return Result{
			packID:  packID,
			size:    size,
			entries: entries,
		}, err
	}

	jobCh := make(chan worker.Job)
	wp := worker.New(listPackWorkers, f, jobCh, ch)

	go func() {
		defer close(jobCh)
		for id := range repo.List(restic.DataFile, done) {
			select {
			case jobCh <- worker.Job{Data: id}:
			case <-done:
				return
			}
		}
	}()

	wp.Wait()
}
