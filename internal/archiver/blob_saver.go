package archiver

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	tomb "gopkg.in/tomb.v2"
)

// Saver allows saving a blob.
type Saver interface {
	SaveBlob(ctx context.Context, t restic.BlobType, data []byte, id restic.ID) (restic.ID, error)
	Index() restic.Index
}

// BlobSaver concurrently saves incoming blobs to the repo.
type BlobSaver struct {
	repo Saver

	m          sync.Mutex
	knownBlobs restic.BlobSet

	ch   chan<- saveBlobJob
	done <-chan struct{}
}

// NewBlobSaver returns a new blob. A worker pool is started, it is stopped
// when ctx is cancelled.
func NewBlobSaver(ctx context.Context, t *tomb.Tomb, repo Saver, workers uint) *BlobSaver {
	ch := make(chan saveBlobJob)
	s := &BlobSaver{
		repo:       repo,
		knownBlobs: restic.NewBlobSet(),
		ch:         ch,
		done:       t.Dying(),
	}

	for i := uint(0); i < workers; i++ {
		t.Go(func() error {
			return s.worker(t.Context(ctx), ch)
		})
	}

	return s
}

// Save stores a blob in the repo. It checks the index and the known blobs
// before saving anything. The second return parameter is true if the blob was
// previously unknown.
func (s *BlobSaver) Save(ctx context.Context, t restic.BlobType, buf *Buffer) FutureBlob {
	ch := make(chan saveBlobResponse, 1)
	select {
	case s.ch <- saveBlobJob{BlobType: t, buf: buf, ch: ch}:
	case <-s.done:
		debug.Log("not sending job, BlobSaver is done")
		close(ch)
		return FutureBlob{ch: ch}
	case <-ctx.Done():
		debug.Log("not sending job, context is cancelled")
		close(ch)
		return FutureBlob{ch: ch}
	}

	return FutureBlob{ch: ch, length: len(buf.Data)}
}

// FutureBlob is returned by SaveBlob and will return the data once it has been processed.
type FutureBlob struct {
	ch     <-chan saveBlobResponse
	length int
	res    saveBlobResponse
}

// Wait blocks until the result is available or the context is cancelled.
func (s *FutureBlob) Wait(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case res, ok := <-s.ch:
		if ok {
			s.res = res
		}
	}
}

// ID returns the ID of the blob after it has been saved.
func (s *FutureBlob) ID() restic.ID {
	return s.res.id
}

// Known returns whether or not the blob was already known.
func (s *FutureBlob) Known() bool {
	return s.res.known
}

// Length returns the length of the blob.
func (s *FutureBlob) Length() int {
	return s.length
}

type saveBlobJob struct {
	restic.BlobType
	buf *Buffer
	ch  chan<- saveBlobResponse
}

type saveBlobResponse struct {
	id    restic.ID
	known bool
}

func (s *BlobSaver) saveBlob(ctx context.Context, t restic.BlobType, buf []byte) (saveBlobResponse, error) {
	id := restic.Hash(buf)
	h := restic.BlobHandle{ID: id, Type: t}

	// check if another goroutine has already saved this blob
	known := false
	s.m.Lock()
	if s.knownBlobs.Has(h) {
		known = true
	} else {
		s.knownBlobs.Insert(h)
		known = false
	}
	s.m.Unlock()

	// blob is already known, nothing to do
	if known {
		return saveBlobResponse{
			id:    id,
			known: true,
		}, nil
	}

	// check if the repo knows this blob
	if s.repo.Index().Has(id, t) {
		return saveBlobResponse{
			id:    id,
			known: true,
		}, nil
	}

	// otherwise we're responsible for saving it
	_, err := s.repo.SaveBlob(ctx, t, buf, id)
	if err != nil {
		return saveBlobResponse{}, err
	}

	return saveBlobResponse{
		id:    id,
		known: false,
	}, nil
}

func (s *BlobSaver) worker(ctx context.Context, jobs <-chan saveBlobJob) error {
	for {
		var job saveBlobJob
		select {
		case <-ctx.Done():
			return nil
		case job = <-jobs:
		}

		res, err := s.saveBlob(ctx, job.BlobType, job.buf.Data)
		if err != nil {
			debug.Log("saveBlob returned error, exiting: %v", err)
			close(job.ch)
			return err
		}
		job.ch <- res
		close(job.ch)
		job.buf.Release()
	}
}
