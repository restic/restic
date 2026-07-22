package object

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/restic/restic/internal/restic"
)

type Writer struct {
	ctx    context.Context
	tpe    restic.BlobType
	chnker restic.Chunker
	saver  restic.BlobSaverAsync

	buf     *restic.BlobBuffer
	flushed bool // writer flushed by user

	lock        sync.Mutex
	content     restic.IDs
	remaining   int
	completed   bool // completeCb was called
	completeErr error

	// statistics
	blobs        int
	size         uint64
	sizeInRepo   uint64
	CompleteBlob func(uint64)
	completeCb   func(ids restic.IDs, err error)
}

var _ io.Writer = &Writer{}

func NewWriter(ctx context.Context, tpe restic.BlobType, chnker restic.Chunker, saver restic.BlobSaverAsync) *Writer {
	w := &Writer{
		ctx:          ctx,
		tpe:          tpe,
		chnker:       chnker,
		saver:        saver,
		content:      restic.IDs{},
		CompleteBlob: func(uint64) {},
	}
	chnker.Reset()
	return w
}

func (w *Writer) Write(data []byte) (int, error) {
	if w.flushed {
		return 0, errors.New("writer already flushed")
	}

	n := len(data)
	for len(data) > 0 {
		if w.ctx.Err() != nil {
			return n - len(data), w.ctx.Err()
		}

		split := w.chnker.NextSplitPoint(data)
		if w.buf == nil {
			w.acquireBuffer()
		}

		if split == -1 {
			w.buf.Data = append(w.buf.Data, data...)
			break
		}

		w.buf.Data = append(w.buf.Data, data[:split]...)
		w.submitBuffer()
		data = data[split:]
	}

	return n, nil
}

func (w *Writer) acquireBuffer() {
	w.buf = w.saver.BlobBufferPool().Get()
}

func (w *Writer) submitBuffer() {
	if w.buf == nil {
		return
	} else if len(w.buf.Data) == 0 {
		w.buf.Release()
		w.buf = nil
		return
	}

	size := len(w.buf.Data)
	w.saver.SaveBlobAsync(w.ctx, w.tpe, w.buf, restic.ID{}, false, w.blobSaved(size))
	w.buf = nil
	w.CompleteBlob(uint64(size))
}

func (w *Writer) blobSaved(size int) func(newID restic.ID, known bool, sizeInRepo int, err error) {
	w.lock.Lock()
	index := len(w.content)
	w.content = append(w.content, restic.ID{})
	w.lock.Unlock()

	return func(newID restic.ID, known bool, sizeInRepo int, err error) {
		w.lock.Lock()
		defer w.lock.Unlock()

		if err != nil {
			if w.completeErr == nil {
				w.completeErr = err
			}
		} else {
			w.content[index] = newID
			if !known {
				w.blobs++
				w.size += uint64(size)
				w.sizeInRepo += uint64(sizeInRepo)
			}
		}

		w.remaining--
		w.tryComplete()
	}
}

// tryComplete must be called with w.lock held.
func (w *Writer) tryComplete() {
	if w.remaining != 0 {
		return
	}
	if w.completed {
		if w.completeErr != nil {
			return
		}
		panic("completed twice")
	}
	w.completed = true

	if w.completeErr != nil {
		w.completeCb(nil, w.completeErr)
		return
	}

	for _, id := range w.content {
		if id.IsNull() {
			panic("completed object with null ID")
		}
	}

	w.completeCb(w.content, nil)
}

func (w *Writer) Flush() (restic.IDs, error) {
	var flushErr error
	ch := make(chan struct{})
	w.FlushAsync(func(_ restic.IDs, err error) {
		flushErr = err
		close(ch)
	})
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case <-ch:
		return w.content, flushErr
	}
}

func (w *Writer) FlushAsync(cb func(ids restic.IDs, err error)) {
	if w.flushed {
		panic("writer already flushed")
	}
	w.flushed = true
	w.submitBuffer()

	w.completeCb = cb
	w.lock.Lock()
	// add number of expected blobs at once to prevent premature completion
	w.remaining += len(w.content)
	w.tryComplete()
	w.lock.Unlock()
}

func (w *Writer) Stats() (int, uint64, uint64) {
	return w.blobs, w.size, w.sizeInRepo
}
