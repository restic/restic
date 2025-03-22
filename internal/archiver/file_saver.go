package archiver

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// saveBlobFn saves a blob to a repo.
type saveBlobFn func(context.Context, restic.BlobType, *buffer, string, func(res saveBlobResponse))

// fileSaver concurrently saves incoming files to the repo.
type fileSaver struct {
	saveFilePool *bufferPool
	saveBlob     saveBlobFn

	pol chunker.Pol

	ch chan<- saveFileJob

	CompleteBlob func(bytes uint64)

	NodeFromFileInfo func(snPath, filename string, meta ToNoder, ignoreXattrListError bool) (*restic.Node, error)

	perFileWorkers uint
	blockSize      uint
}

// newFileSaver returns a new file saver. A worker pool with fileWorkers is
// started, it is stopped when ctx is cancelled.
func newFileSaver(ctx context.Context, wg *errgroup.Group, save saveBlobFn, pol chunker.Pol, fileWorkers, blobWorkers, blockSize uint) *fileSaver {
	ch := make(chan saveFileJob)

	debug.Log("new file saver with %v file workers and %v blob workers", fileWorkers, blobWorkers)

	poolSize := fileWorkers + blobWorkers

	var perFileWorkers uint
	if blockSize == 0 {
		perFileWorkers = 1
	} else {
		perFileWorkers = fileWorkers
		fileWorkers = 1
	}

	s := &fileSaver{
		saveBlob:     save,
		saveFilePool: newBufferPool(int(poolSize), chunker.MaxSize),
		pol:          pol,
		ch:           ch,

		CompleteBlob: func(uint64) {},

		perFileWorkers: perFileWorkers,
		blockSize:      blockSize,
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
type fileCompleteFunc func(*restic.Node, ItemStats)

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
func (s *fileSaver) saveFile(ctx context.Context, chnkers []*chunker.Chunker, snPath string, target string, f fs.File, start func(), finishReading func(), finish func(res futureNodeResult)) {
	start()

	fnr := futureNodeResult{
		snPath: snPath,
		target: target,
	}
	var lock sync.Mutex
	isCompleted := false

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

	if node.Type != restic.NodeTypeFile {
		_ = f.Close()
		completeError(errors.Errorf("node type %q is wrong", node.Type))
		return
	}

	fileSize := uint(node.Size)

	remaining := 0
	var blockCount uint
	if s.blockSize == 0 {
		blockCount = 0
	} else {
		// simply an integer division but with rounding up
		blockCount = (fileSize + s.blockSize) / s.blockSize
	}

	contentByBlock := make([]*[]restic.ID, 0, blockCount)

	completeBlob := func(blobLength, blobSize, blobSizeInRepo uint64, blobKnown bool) {
		lock.Lock()
		defer lock.Unlock()

		fnr.node.Size += blobSize

		if !blobKnown {
			fnr.stats.DataBlobs++
			fnr.stats.DataSize += blobLength
			fnr.stats.DataSizeInRepo += blobSizeInRepo
		}

		remaining--
		if remaining == 0 && fnr.err == nil {
			if isCompleted {
				panic("completed twice")
			}
			isCompleted = true

			if len(contentByBlock) == 1 {
				// don't copy the content
				fnr.node.Content = *contentByBlock[0]
			} else {
				// count the total amount of blobs in order to preallocate memory
				totalBlobs := 0
				for _, blockContent := range contentByBlock {
					totalBlobs += len(*blockContent)
				}

				fnr.node.Content = make([]restic.ID, 0, totalBlobs)
				for _, blockContent := range contentByBlock {
					fnr.node.Content = append(fnr.node.Content, *blockContent...)
				}
			}

			for _, id := range fnr.node.Content {
				if id.IsNull() {
					panic("completed file with null ID")
				}
			}

			finish(fnr)
		}
	}

	// reset node size, as we're going to calculate it
	node.Size = 0
	fnr.node = node

	jobs := make(chan processBlobJob)
	results := make(chan int)
	wg, wgCtx := errgroup.WithContext(ctx)

	for i := 0; i < len(chnkers); i++ {
		chnkerNum := i
		wg.Go(func() error {
			return s.processBlobWorker(jobs, results, chnkers[chnkerNum], wgCtx, target, f, &lock, completeBlob)
		})
	}

	wg.Go(func() error {
		defer close(jobs)

		offsetStart := uint(0)
		for offsetStart < fileSize || fileSize == 0 {
			nextOffsetStart := offsetStart + s.blockSize
			var blockSize int64
			if nextOffsetStart < fileSize || s.blockSize == 0 {
				blockSize = int64(s.blockSize)
			} else {
				// read the remains in case the file has grown in size
				blockSize = math.MaxInt64
			}

			targetContent := make([]restic.ID, 0)
			contentByBlock = append(contentByBlock, &targetContent)
			select {
			case jobs <- processBlobJob{&targetContent, int64(offsetStart), blockSize}:
			case <-ctx.Done():
				return ctx.Err()
			}

			if s.blockSize == 0 || fileSize == 0 {
				break
			}
			offsetStart = nextOffsetStart
		}
		return nil
	})

	go func() {
		wg.Wait()
		close(results)
	}()

	totalChunks := 0

	for result := range results {
		totalChunks += result
	}

	// at this point wg has been awaited - but we need to process errors
	err = wg.Wait()
	if err != nil {
		_ = f.Close()
		completeError(err)
		return
	}

	err = f.Close()
	if err != nil {
		completeError(err)
		return
	}

	lock.Lock()
	// require one additional completeFuture() call to ensure that the future only completes
	// after reaching the end of this method
	remaining += totalChunks + 1
	lock.Unlock()
	finishReading()
	completeBlob(0, 0, 0, true)
}

type processBlobJob struct {
	targetContent *[]restic.ID
	offsetStart   int64
	blockSize     int64
}

func (s *fileSaver) processBlobWorker(
	jobs <-chan processBlobJob,
	results chan<- int,
	chnker *chunker.Chunker,
	ctx context.Context,
	target string,
	f fs.File,
	lock *sync.Mutex,
	completeBlob func(uint64, uint64, uint64, bool),
) error {

	for {
		var job processBlobJob
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case job, ok = <-jobs:
			if !ok {
				return nil
			}
		}
		var reader io.Reader
		if job.blockSize == 0 {
			// '0' indicates that we do not cut the file in parts
			reader = f
		} else {
			reader = io.NewSectionReader(f, job.offsetStart, job.blockSize)
		}
		chunksCount, err := s.processBlobs(ctx, target, reader, lock, completeBlob, chnker, job.targetContent)
		if err != nil {
			return err
		}
		results <- chunksCount
	}
}

func (s *fileSaver) processBlobs(
	ctx context.Context,
	target string,
	f io.Reader,
	lock *sync.Mutex,
	completeBlob func(uint64, uint64, uint64, bool),
	chnker *chunker.Chunker,
	targetContent *[]restic.ID,
) (int, error) {

	var chunksCount int
	chnker.Reset(f, s.pol)

	for {
		buf := s.saveFilePool.Get()
		chunk, err := chnker.Next(buf.Data)
		if err == io.EOF {
			buf.Release()
			break
		}

		if err != nil {
			return 0, err
		}

		buf.Data = chunk.Data

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}

		// redefinition of pos and on each iteration is needed as we pass it to saveBlob cb
		pos := chunksCount
		size := uint64(chunk.Length)

		// add a place to store the saveBlob result
		lock.Lock()
		*targetContent = append(*targetContent, restic.ID{})
		lock.Unlock()

		s.saveBlob(ctx, restic.DataBlob, buf, target, func(sbr saveBlobResponse) {
			lock.Lock()
			(*targetContent)[pos] = sbr.id
			lock.Unlock()

			completeBlob(uint64(sbr.length), size, uint64(sbr.sizeInRepo), sbr.known)
		})
		chunksCount++

		// test if the context has been cancelled, return the error
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}

		s.CompleteBlob(uint64(len(chunk.Data)))
	}
	return chunksCount, nil
}

func (s *fileSaver) worker(ctx context.Context, jobs <-chan saveFileJob) {
	// each worker has a fixed amount of chunkers which are reused for each file (because they contain a rather large buffer)
	chunkersAmount := int(s.perFileWorkers)
	chnkers := make([]*chunker.Chunker, chunkersAmount)
	for i := 0; i < chunkersAmount; i++ {
		chnkers[i] = chunker.New(nil, s.pol)
	}

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

		s.saveFile(ctx, chnkers, job.snPath, job.target, job.file, job.start, func() {
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
