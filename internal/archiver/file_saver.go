package archiver

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// fileSaver concurrently saves incoming files to the repo.
type fileSaver struct {
	saveFilePool *bufferPool
	uploader     restic.BlobSaverAsync

	pol chunker.Pol

	ch chan<- saveFileJob

	CompleteBlob func(bytes uint64)

	NodeFromFileInfo func(snPath, filename string, meta ToNoder, ignoreXattrListError bool) (*data.Node, error)
}

// newFileSaver returns a new file saver. A worker pool with fileWorkers is
// started, it is stopped when ctx is cancelled.
func newFileSaver(ctx context.Context, wg *errgroup.Group, uploader restic.BlobSaverAsync, pol chunker.Pol, fileWorkers uint) *fileSaver {
	ch := make(chan saveFileJob)
	debug.Log("new file saver with %v file workers", fileWorkers)

	s := &fileSaver{
		uploader:     uploader,
		saveFilePool: newBufferPool(chunker.MaxSize),
		pol:          pol,
		ch:           ch,

		CompleteBlob: func(uint64) {},
	}

	for i := uint(0); i < fileWorkers; i++ {
		wg.Go(func() error {
			s.worker(ctx, ch)
			return nil
		})
	}

	return s
}

func (s *fileSaver) TriggerShutdown() {
	close(s.ch)
}

// fileCompleteFunc is called when the file has been saved.
type fileCompleteFunc func(*data.Node, ItemStats)

// Save stores the file f and returns the data once it has been completed. The
// file is closed by Save. completeReading is only called if the file was read
// successfully. complete is always called. If completeReading is called, then
// this will always happen before calling complete.
func (s *fileSaver) Save(ctx context.Context, snPath string, target string, file fs.File, start func(), completeReading func(), complete fileCompleteFunc) futureNode {
	fn, ch := newFutureNode()
	job := saveFileJob{
		snPath: snPath,
		target: target,
		file:   file,
		ch:     ch,

		start:           start,
		completeReading: completeReading,
		complete:        complete,
	}

	select {
	case s.ch <- job:
	case <-ctx.Done():
		debug.Log("not sending job, context is cancelled: %v", ctx.Err())
		_ = file.Close()
		close(ch)
	}

	return fn
}

type saveFileJob struct {
	snPath string
	target string
	file   fs.File
	ch     chan<- futureNodeResult

	start           func()
	completeReading func()
	complete        fileCompleteFunc
}

// saveFile stores the file f in the repo, then closes it.
func (s *fileSaver) saveFile(ctx context.Context, chnker *chunker.Chunker, snPath string, target string, f fs.File, start func(), finishReading func(), finish func(res futureNodeResult)) {
	start()

	fnr := futureNodeResult{
		snPath: snPath,
		target: target,
	}
	var lock sync.Mutex
	remaining := 0
	isCompleted := false

	completeBlob := func() {
		lock.Lock()
		defer lock.Unlock()

		remaining--
		if remaining == 0 && fnr.err == nil {
			if isCompleted {
				panic("completed twice")
			}
			for _, id := range fnr.node.Content {
				if id.IsNull() {
					panic("completed file with null ID")
				}
			}
			isCompleted = true
			finish(fnr)
		}
	}
	completeError := func(err error) {
		lock.Lock()
		defer lock.Unlock()

		if fnr.err == nil {
			if isCompleted {
				panic("completed twice")
			}
			isCompleted = true
			fnr.err = fmt.Errorf("failed to save %v: %w", target, err)
			fnr.node = nil
			fnr.stats = ItemStats{}
			finish(fnr)
		}
	}

	debug.Log("%v", snPath)

	node, err := s.NodeFromFileInfo(snPath, target, f, false)
	if err != nil {
		_ = f.Close()
		completeError(err)
		return
	}

	if node.Type != data.NodeTypeFile {
		_ = f.Close()
		completeError(errors.Errorf("node type %q is wrong", node.Type))
		return
	}

	// reuse the chunker
	chnker.Reset(f, s.pol)

	node.Content = []restic.ID{}
	node.Size = 0
	var idx int
	for {
		buf := s.saveFilePool.Get()
		chunk, err := chnker.Next(buf.Data)
		if err == io.EOF {
			buf.Release()
			break
		}

		buf.Data = chunk.Data
		node.Size += uint64(chunk.Length)

		if err != nil {
			_ = f.Close()
			completeError(err)
			return
		}
		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			completeError(ctx.Err())
			return
		}

		// add a place to store the saveBlob result
		pos := idx

		lock.Lock()
		node.Content = append(node.Content, restic.ID{})
		lock.Unlock()

		s.uploader.SaveBlobAsync(ctx, restic.DataBlob, buf.Data, restic.ID{}, false, func(newID restic.ID, known bool, sizeInRepo int, err error) {
			defer buf.Release()
			if err != nil {
				completeError(err)
				return
			}

			lock.Lock()
			if !known {
				fnr.stats.DataBlobs++
				fnr.stats.DataSize += uint64(len(buf.Data))
				fnr.stats.DataSizeInRepo += uint64(sizeInRepo)
			}
			node.Content[pos] = newID
			lock.Unlock()

			completeBlob()
		})
		idx++

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			completeError(ctx.Err())
			return
		}

		s.CompleteBlob(uint64(len(chunk.Data)))
	}

	err = f.Close()
	if err != nil {
		completeError(err)
		return
	}

	fnr.node = node
	lock.Lock()
	// require one additional completeFuture() call to ensure that the future only completes
	// after reaching the end of this method
	remaining += idx + 1
	lock.Unlock()
	finishReading()
	completeBlob()
}

func (s *fileSaver) worker(ctx context.Context, jobs <-chan saveFileJob) {
	// a worker has one chunker which is reused for each file (because it contains a rather large buffer)
	chnker := chunker.New(nil, s.pol)

	for {
		var job saveFileJob
		var ok bool
		select {
		case <-ctx.Done():
			return
		case job, ok = <-jobs:
			if !ok {
				return
			}
		}

		s.saveFile(ctx, chnker, job.snPath, job.target, job.file, job.start, func() {
			if job.completeReading != nil {
				job.completeReading()
			}
		}, func(res futureNodeResult) {
			if job.complete != nil {
				job.complete(res.node, res.stats)
			}
			job.ch <- res
			close(job.ch)
		})
	}
}
