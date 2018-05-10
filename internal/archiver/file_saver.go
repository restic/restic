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
)

// Goer starts a function in a goroutine.
type Goer interface {
	Go(func() error)
}

// FutureFile is returned by Save and will return the data once it
// has been processed.
type FutureFile struct {
	ch  <-chan saveFileResponse
	res saveFileResponse
}

func (s *FutureFile) wait() {
	res, ok := <-s.ch
	if ok {
		s.res = res
	}
}

// Node returns the node once it is available.
func (s *FutureFile) Node() *restic.Node {
	s.wait()
	return s.res.node
}

// Stats returns the stats for the file once they are available.
func (s *FutureFile) Stats() ItemStats {
	s.wait()
	return s.res.stats
}

// Err returns the error in case an error occurred.
func (s *FutureFile) Err() error {
	s.wait()
	return s.res.err
}

// FileSaver concurrently saves incoming files to the repo.
type FileSaver struct {
	fs           fs.FS
	blobSaver    *BlobSaver
	saveFilePool *BufferPool

	pol chunker.Pol

	ch chan<- saveFileJob

	CompleteBlob func(filename string, bytes uint64)

	NodeFromFileInfo func(filename string, fi os.FileInfo) (*restic.Node, error)
}

// NewFileSaver returns a new file saver. A worker pool with fileWorkers is
// started, it is stopped when ctx is cancelled.
func NewFileSaver(ctx context.Context, g Goer, fs fs.FS, blobSaver *BlobSaver, pol chunker.Pol, fileWorkers, blobWorkers uint) *FileSaver {
	ch := make(chan saveFileJob)

	debug.Log("new file saver with %v file workers and %v blob workers", fileWorkers, blobWorkers)

	poolSize := fileWorkers + blobWorkers

	s := &FileSaver{
		fs:           fs,
		blobSaver:    blobSaver,
		saveFilePool: NewBufferPool(ctx, int(poolSize), chunker.MaxSize),
		pol:          pol,
		ch:           ch,

		CompleteBlob: func(string, uint64) {},
	}

	for i := uint(0); i < fileWorkers; i++ {
		g.Go(func() error {
			s.worker(ctx, ch)
			return nil
		})
	}

	return s
}

// CompleteFunc is called when the file has been saved.
type CompleteFunc func(*restic.Node, ItemStats)

// Save stores the file f and returns the data once it has been completed. The
// file is closed by Save.
func (s *FileSaver) Save(ctx context.Context, snPath string, file fs.File, fi os.FileInfo, start func(), complete CompleteFunc) FutureFile {
	ch := make(chan saveFileResponse, 1)
	job := saveFileJob{
		snPath:   snPath,
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
	}

	return FutureFile{ch: ch}
}

type saveFileJob struct {
	snPath   string
	file     fs.File
	fi       os.FileInfo
	ch       chan<- saveFileResponse
	complete CompleteFunc
	start    func()
}

type saveFileResponse struct {
	node  *restic.Node
	stats ItemStats
	err   error
}

// saveFile stores the file f in the repo, then closes it.
func (s *FileSaver) saveFile(ctx context.Context, chnker *chunker.Chunker, snPath string, f fs.File, fi os.FileInfo, start func()) saveFileResponse {
	start()

	stats := ItemStats{}

	debug.Log("%v", snPath)

	node, err := s.NodeFromFileInfo(f.Name(), fi)
	if err != nil {
		_ = f.Close()
		return saveFileResponse{err: err}
	}

	if node.Type != "file" {
		_ = f.Close()
		return saveFileResponse{err: errors.Errorf("node type %q is wrong", node.Type)}
	}

	// reuse the chunker
	chnker.Reset(f, s.pol)

	var results []FutureBlob

	node.Content = []restic.ID{}
	var size uint64
	for {
		buf := s.saveFilePool.Get()
		chunk, err := chnker.Next(buf.Data)
		if errors.Cause(err) == io.EOF {
			buf.Release()
			break
		}

		buf.Data = chunk.Data

		size += uint64(chunk.Length)

		if err != nil {
			_ = f.Close()
			return saveFileResponse{err: err}
		}

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			return saveFileResponse{err: ctx.Err()}
		}

		res := s.blobSaver.Save(ctx, restic.DataBlob, buf)
		results = append(results, res)

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			_ = f.Close()
			return saveFileResponse{err: ctx.Err()}
		}

		s.CompleteBlob(f.Name(), uint64(len(chunk.Data)))
	}

	err = f.Close()
	if err != nil {
		return saveFileResponse{err: err}
	}

	for _, res := range results {
		res.Wait(ctx)
		if !res.Known() {
			stats.DataBlobs++
			stats.DataSize += uint64(res.Length())
		}

		node.Content = append(node.Content, res.ID())
	}

	node.Size = size

	return saveFileResponse{
		node:  node,
		stats: stats,
	}
}

func (s *FileSaver) worker(ctx context.Context, jobs <-chan saveFileJob) {
	// a worker has one chunker which is reused for each file (because it contains a rather large buffer)
	chnker := chunker.New(nil, s.pol)

	for {
		var job saveFileJob
		select {
		case <-ctx.Done():
			return
		case job = <-jobs:
		}

		res := s.saveFile(ctx, chnker, job.snPath, job.file, job.fi, job.start)
		if job.complete != nil {
			job.complete(res.node, res.stats)
		}
		job.ch <- res
		close(job.ch)
	}
}
