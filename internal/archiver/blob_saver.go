package archiver

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// saver allows saving a blob.
type saver interface {
	SaveBlob(ctx context.Context, t restic.BlobType, data []byte, id restic.ID, storeDuplicate bool) (restic.ID, bool, int, error)
}

// blobSaver concurrently saves incoming blobs to the repo.
type blobSaver struct {
	repo saver
	ch   chan<- saveBlobJob
}

// newBlobSaver returns a new blob. A worker pool is started, it is stopped
// when ctx is cancelled.
func newBlobSaver(ctx context.Context, wg *errgroup.Group, repo saver, workers uint) *blobSaver {
	ch := make(chan saveBlobJob)
	s := &blobSaver{
		repo: repo,
		ch:   ch,
	}

	for i := uint(0); i < workers; i++ {
		wg.Go(func() error {
			return s.worker(ctx, ch)
		})
	}

	return s
}

func (s *blobSaver) TriggerShutdown() {
	close(s.ch)
}

// Save stores a blob in the repo. It checks the index and the known blobs
// before saving anything. It takes ownership of the buffer passed in.
func (s *blobSaver) Save(ctx context.Context, t restic.BlobType, buf *buffer, filename string, cb func(res saveBlobResponse)) {
	select {
	case s.ch <- saveBlobJob{BlobType: t, buf: buf, fn: filename, cb: cb}:
	case <-ctx.Done():
		debug.Log("not sending job, context is cancelled")
	}
}

type saveBlobJob struct {
	restic.BlobType
	buf *buffer
	fn  string
	cb  func(res saveBlobResponse)
}

type saveBlobResponse struct {
	id         restic.ID
	length     int
	sizeInRepo int
	known      bool
}

func (s *blobSaver) saveBlob(ctx context.Context, t restic.BlobType, buf []byte) (saveBlobResponse, error) {
	id, known, sizeInRepo, err := s.repo.SaveBlob(ctx, t, buf, restic.ID{}, false)

	if err != nil {
		return saveBlobResponse{}, err
	}

	return saveBlobResponse{
		id:         id,
		length:     len(buf),
		sizeInRepo: sizeInRepo,
		known:      known,
	}, nil
}

func (s *blobSaver) worker(ctx context.Context, jobs <-chan saveBlobJob) error {
	for {
		var job saveBlobJob
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case job, ok = <-jobs:
			if !ok {
				return nil
			}
		}

		res, err := s.saveBlob(ctx, job.BlobType, job.buf.Data)
		if err != nil {
			debug.Log("saveBlob returned error, exiting: %v", err)
			return fmt.Errorf("failed to save blob from file %q: %w", job.fn, err)
		}
		job.cb(res)
		job.buf.Release()
	}
}
