package list

import (
	"context"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/worker"
)

const listPackWorkers = 10

// Lister combines lists packs in a repo and blobs in a pack.
type Lister interface {
	List(context.Context, restic.FileType, func(restic.ID, int64) error) error
	ListPack(context.Context, restic.ID, int64) ([]restic.Blob, int64, error)
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

// Size returns the size of the pack.
func (l Result) Size() int64 {
	return l.size
}

// Entries returns a list of all blobs saved in the pack.
func (l Result) Entries() []restic.Blob {
	return l.entries
}

// AllPacks sends the contents of all packs to ch.
func AllPacks(ctx context.Context, repo Lister, ignorePacks restic.IDSet, ch chan<- worker.Job) {
	type fileInfo struct {
		id   restic.ID
		size int64
	}

	f := func(ctx context.Context, job worker.Job) (interface{}, error) {
		packInfo := job.Data.(fileInfo)
		entries, size, err := repo.ListPack(ctx, packInfo.id, packInfo.size)

		return Result{
			packID:  packInfo.id,
			size:    size,
			entries: entries,
		}, err
	}

	jobCh := make(chan worker.Job)
	wp := worker.New(ctx, listPackWorkers, f, jobCh, ch)

	go func() {
		defer close(jobCh)

		_ = repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
			if ignorePacks.Has(id) {
				return nil
			}

			select {
			case jobCh <- worker.Job{Data: fileInfo{id: id, size: size}, Result: Result{packID: id}}:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}()

	wp.Wait()
}
