package rechunker

import (
	"context"
	"io"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type FileResult struct {
	dstBlobs          restic.IDs
	addedToRepository uint64
}
type Worker struct {
	pool *BufferPool

	chunker    *chunker.Chunker
	pol        chunker.Pol
	downloader restic.BlobLoader
	uploader   restic.BlobSaver

	newCursor    func(blobs restic.IDs) cursor
	updateCursor func(c cursor, numBytes uint) (cursor, error)
}
type WorkerConfig struct {
	Pol chunker.Pol

	Downloader restic.BlobLoader
	Uploader   restic.BlobSaver
	BufferPool *BufferPool

	NewCursor    func(blobs restic.IDs) cursor
	UpdateCursor func(c cursor, numBytes uint) (cursor, error)
}

func NewWorker(cfg WorkerConfig) *Worker {
	if cfg.BufferPool == nil {
		cfg.BufferPool = NewBufferPool(3)
	}
	return &Worker{
		pool: cfg.BufferPool,

		chunker:    chunker.New(nil, cfg.Pol),
		pol:        cfg.Pol,
		downloader: cfg.Downloader,
		uploader:   cfg.Uploader,

		newCursor:    cfg.NewCursor,
		updateCursor: cfg.UpdateCursor,
	}
}

func (w *Worker) RunFile(ctx context.Context, srcBlobs restic.IDs, p *Progress) (FileResult, error) {
	buf := w.pool.Get()

	// setup reader
	reader := NewBlobSequenceReader(ctx, srcBlobs, w.downloader, buf)

	// Run worker pipeline (reader and writer)
	wg, ctx := errgroup.WithContext(ctx)

	chChunk := make(chan chunker.Chunk)  // chunk passing channel from reader to writer
	chResult := make(chan FileResult, 1) // file rechunk result channel

	// Run reader goroutine
	w.runReader(ctx, wg, srcBlobs, reader, chChunk)

	// Run writer goroutine
	w.runWriter(ctx, wg, chChunk, chResult, p)

	if err := wg.Wait(); err != nil {
		return FileResult{}, err
	}

	result := <-chResult

	w.pool.Put(buf)

	return result, nil
}

func (w *Worker) runReader(ctx context.Context, wg *errgroup.Group, srcBlobs restic.IDs, reader *BlobSequenceReader, out chan<- chunker.Chunk) {
	debug.Log("Starting reader goroutine")
	wg.Go(func() error {
		defer close(out)

		w.chunker.Reset(reader, w.pol)

		var c cursor
		if w.newCursor != nil {
			c = w.newCursor(srcBlobs)
		}

		for {
			// bring buffer from bufferPool
			buf := w.pool.Get()

			// rechunk with new parameter
			chunk, err := w.chunker.Next(buf)
			if err == io.EOF { // reached EOF; all done
				w.pool.Put(buf)
				return nil
			}
			if err != nil {
				return err
			}

			if w.updateCursor != nil {
				c, err = w.updateCursor(c, chunk.Length)
				if err != nil {
					return err
				}
			}

			// send a rechunked chunk to the writer
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- chunk:
				debug.Log("Sending a new chunk of size %v to writer", chunk.Length)
			}
		}
	})
}

func (w *Worker) runWriter(ctx context.Context, wg *errgroup.Group, in <-chan chunker.Chunk, out chan<- FileResult, p *Progress) {
	debug.Log("Starting writer goroutine")
	wg.Go(func() error {
		defer close(out)

		dstBlobs := restic.IDs{}
		var addedSize uint64

		for {
			// receive chunk from the reader
			var c chunker.Chunk
			var ok bool
			select {
			case <-ctx.Done():
				return ctx.Err()
			case c, ok = <-in:
				if !ok { // EOF
					out <- FileResult{
						dstBlobs:          dstBlobs,
						addedToRepository: addedSize,
					}
					return nil
				}
			}

			// save chunk to destination repo
			dstBlobID, known, size, err := w.uploader.SaveBlob(ctx, restic.DataBlob, c.Data, restic.ID{}, false)
			if err != nil {
				return err
			}
			if !known {
				addedSize += uint64(size)
				debug.Log("Stored new dst chunk %v into dstRepo", dstBlobID.Str())
			}

			if p != nil {
				p.AddBlob(uint64(c.Length))
			}

			// recycle used buffer into bufferPool
			w.pool.Put(c.Data)

			dstBlobs = append(dstBlobs, dstBlobID)
		}
	})
}

type BlobSequenceReader struct {
	ctx        context.Context
	downloader restic.BlobLoader

	blobs restic.IDs

	data []byte // data of the current blob being read
	buf  []byte // reused buffer space
}

func NewBlobSequenceReader(ctx context.Context, blobs restic.IDs, downloader restic.BlobLoader, buf []byte) *BlobSequenceReader {
	return &BlobSequenceReader{
		ctx:        ctx,
		blobs:      blobs,
		downloader: downloader,
		buf:        buf,
	}
}

func (r *BlobSequenceReader) Read(p []byte) (n int, err error) {
	if len(r.data) == 0 {
		// out of data; load the next blob
		if len(r.blobs) == 0 {
			return 0, io.EOF
		}

		// bring the blob data from backend
		r.data, err = r.downloader.LoadBlob(r.ctx, restic.DataBlob, r.blobs[0], r.buf)
		if err != nil {
			return 0, err
		}

		r.blobs = r.blobs[1:]
	}

	// copy data from currentBuf to p
	n = copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type BufferPool struct {
	c chan []byte
}

func NewBufferPool(cap int) *BufferPool {
	return &BufferPool{
		c: make(chan []byte, cap),
	}
}

func (p *BufferPool) Get() []byte {
	select {
	case buf := <-p.c:
		return buf[:0]
	default:
		debug.Log("Allocating new buffer")
		return make([]byte, 0, chunker.MaxSize)
	}
}

func (p *BufferPool) Put(buf []byte) {
	select {
	case p.c <- buf:
	default:
		debug.Log("bufferPool is full; discarding the buffer")
	}
}
