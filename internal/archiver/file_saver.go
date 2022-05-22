package archiver

import (
	"context"
	"io"
	"os"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// SaveBlobFn saves a blob to a repo.
type SaveBlobFn func(context.Context, restic.BlobType, *Buffer) FutureBlob

// FileSaver concurrently saves incoming files to the repo.
type FileSaver struct {
	saveFilePool *BufferPool
	saveBlob     SaveBlobFn

	pol chunker.Pol

	ch chan<- saveFileJob

	CompleteBlob func(filename string, bytes uint64)

	NodeFromFileInfo func(filename string, fi os.FileInfo) (*restic.Node, error)
}

// NewFileSaver returns a new file saver. A worker pool with fileWorkers is
// started, it is stopped when ctx is cancelled.
func NewFileSaver(ctx context.Context, wg *errgroup.Group, save SaveBlobFn, pol chunker.Pol, fileWorkers, blobWorkers uint) *FileSaver {
	ch := make(chan saveFileJob)

	debug.Log("new file saver with %v file workers and %v blob workers", fileWorkers, blobWorkers)

	poolSize := fileWorkers + blobWorkers

	s := &FileSaver{
		saveBlob:     save,
		saveFilePool: NewBufferPool(int(poolSize), chunker.MaxSize),
		pol:          pol,
		ch:           ch,

		CompleteBlob: func(string, uint64) {},
	}

	for i := uint(0); i < fileWorkers; i++ {
		wg.Go(func() error {
			s.worker(ctx, ch)
			return nil
		})
	}

	return s
}

func (s *FileSaver) TriggerShutdown() {
	close(s.ch)
}

// CompleteFunc is called when the file has been saved.
type CompleteFunc func(*restic.Node, ItemStats)

// Save stores the file f and returns the data once it has been completed. The
// file is closed by Save.
func (s *FileSaver) Save(ctx context.Context, snPath string, target string, file fs.File, fi os.FileInfo, start func(), complete CompleteFunc) FutureNode {
	fn, ch := newFutureNode()
	job := saveFileJob{
		snPath:   snPath,
		target:   target,
		file:     file,
		fi:       fi,
		start:    start,
		complete: complete,
		ch:       ch,
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
	snPath   string
	target   string
	file     fs.File
	fi       os.FileInfo
	ch       chan<- futureNodeResult
	complete CompleteFunc
	start    func()
}

// saveFile stores the file f in the repo, then closes it.
func (s *FileSaver) saveFile(ctx context.Context, chnker *chunker.Chunker, snPath string, target string, f fs.File, fi os.FileInfo, start func()) futureNodeResult {
	start()

	stats := ItemStats{}
	fnr := futureNodeResult{
		snPath: snPath,
		target: target,
	}

	debug.Log("%v", snPath)

	node, err := s.NodeFromFileInfo(f.Name(), fi)
	if err != nil {
		_ = f.Close()
		fnr.err = err
		return fnr
	}

	if node.Type != "file" {
		_ = f.Close()
		fnr.err = errors.Errorf("node type %q is wrong", node.Type)
		return fnr
	}

	// reuse the chunker
	chnker.Reset(f, s.pol)

	var results []FutureBlob
	complete := func(sbr SaveBlobResponse) {
		if !sbr.known {
			stats.DataBlobs++
			stats.DataSize += uint64(sbr.length)
			stats.DataSizeInRepo += uint64(sbr.sizeInRepo)
		}

		node.Content = append(node.Content, sbr.id)
	}

	node.Content = []restic.ID{}
	var size uint64
	for {
		buf := s.saveFilePool.Get()
		chunk, err := chnker.Next(buf.Data)
		if err == io.EOF {
			buf.Release()
			break
		}

		buf.Data = chunk.Data

		size += uint64(chunk.Length)

		if err != nil {
			_ = f.Close()
			fnr.err = err
			return fnr
		}

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			fnr.err = ctx.Err()
			return fnr
		}

		res := s.saveBlob(ctx, restic.DataBlob, buf)
		results = append(results, res)

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			fnr.err = ctx.Err()
			return fnr
		}

		s.CompleteBlob(f.Name(), uint64(len(chunk.Data)))

		// collect already completed blobs
		for len(results) > 0 {
			sbr := results[0].Poll()
			if sbr == nil {
				break
			}
			results[0] = FutureBlob{}
			results = results[1:]
			complete(*sbr)
		}
	}

	err = f.Close()
	if err != nil {
		fnr.err = err
		return fnr
	}

	for i, res := range results {
		results[i] = FutureBlob{}
		sbr := res.Take(ctx)
		complete(sbr)
	}

	node.Size = size
	fnr.node = node
	fnr.stats = stats
	return fnr
}

func (s *FileSaver) worker(ctx context.Context, jobs <-chan saveFileJob) {
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

		res := s.saveFile(ctx, chnker, job.snPath, job.target, job.file, job.fi, job.start)
		if job.complete != nil {
			job.complete(res.node, res.stats)
		}
		job.ch <- res
		close(job.ch)
	}
}
