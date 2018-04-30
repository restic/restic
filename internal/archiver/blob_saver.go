package archiver

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/restic"
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

	ch chan<- saveBlobJob
	wg sync.WaitGroup
}

// NewBlobSaver returns a new blob. A worker pool is started, it is stopped
// when ctx is cancelled.
func NewBlobSaver(ctx context.Context, repo Saver, workers uint) *BlobSaver {
	ch := make(chan saveBlobJob)
	s := &BlobSaver{
		repo:       repo,
		knownBlobs: restic.NewBlobSet(),
		ch:         ch,
	}

	for i := uint(0); i < workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, &s.wg, ch)
	}

	return s
}

// Save stores a blob in the repo. It checks the index and the known blobs
// before saving anything. The second return parameter is true if the blob was
// previously unknown.
func (s *BlobSaver) Save(ctx context.Context, t restic.BlobType, buf *Buffer) FutureBlob {
	ch := make(chan saveBlobResponse, 1)
	s.ch <- saveBlobJob{BlobType: t, buf: buf, ch: ch}

	return FutureBlob{ch: ch, length: len(buf.Data)}
}

// FutureBlob is returned by SaveBlob and will return the data once it has been processed.
type FutureBlob struct {
	ch     <-chan saveBlobResponse
	length int
	res    saveBlobResponse
}

func (s *FutureBlob) wait() {
	res, ok := <-s.ch
	if ok {
		s.res = res
	}
}

// ID returns the ID of the blob after it has been saved.
func (s *FutureBlob) ID() restic.ID {
	s.wait()
	return s.res.id
}

// Known returns whether or not the blob was already known.
func (s *FutureBlob) Known() bool {
	s.wait()
	return s.res.known
}

// Err returns the error which may have occurred during save.
func (s *FutureBlob) Err() error {
	s.wait()
	return s.res.err
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
	err   error
}

func (s *BlobSaver) saveBlob(ctx context.Context, t restic.BlobType, buf []byte) saveBlobResponse {
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
		}
	}

	// check if the repo knows this blob
	if s.repo.Index().Has(id, t) {
		return saveBlobResponse{
			id:    id,
			known: true,
		}
	}

	// otherwise we're responsible for saving it
	_, err := s.repo.SaveBlob(ctx, t, buf, id)
	return saveBlobResponse{
		id:    id,
		known: false,
		err:   err,
	}
}

func (s *BlobSaver) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan saveBlobJob) {
	defer wg.Done()
	for {
		var job saveBlobJob
		select {
		case <-ctx.Done():
			return
		case job = <-jobs:
		}

		job.ch <- s.saveBlob(ctx, job.BlobType, job.buf.Data)
		close(job.ch)
		job.buf.Release()
	}
}
