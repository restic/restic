package archiver

import (
	"context"
	"sync"
)

// Buffer is a reusable buffer. After the buffer has been used, Release should
// be called so the underlying slice is put back into the pool.
type Buffer struct {
	Data []byte
	Put  func(*Buffer)
}

// Release puts the buffer back into the pool it came from.
func (b *Buffer) Release() {
	if b.Put != nil {
		b.Put(b)
	}
}

// BufferPool implements a limited set of reusable buffers.
type BufferPool struct {
	ch          chan *Buffer
	chM         sync.Mutex
	defaultSize int
	clearOnce   sync.Once
}

// NewBufferPool initializes a new buffer pool. When the context is cancelled,
// all buffers are released. The pool stores at most max items. New buffers are
// created with defaultSize, buffers that are larger are released and not put
// back.
func NewBufferPool(ctx context.Context, max int, defaultSize int) *BufferPool {
	b := &BufferPool{
		ch:          make(chan *Buffer, max),
		defaultSize: defaultSize,
	}
	go func() {
		<-ctx.Done()
		b.clear()
	}()
	return b
}

// Get returns a new buffer, either from the pool or newly allocated.
func (pool *BufferPool) Get() *Buffer {
	pool.chM.Lock()
	defer pool.chM.Unlock()
	select {
	case buf := <-pool.ch:
		return buf
	default:
	}

	b := &Buffer{
		Put:  pool.Put,
		Data: make([]byte, pool.defaultSize),
	}

	return b
}

// Put returns a buffer to the pool for reuse.
func (pool *BufferPool) Put(b *Buffer) {
	if cap(b.Data) > pool.defaultSize {
		return
	}

	pool.chM.Lock()
	defer pool.chM.Unlock()
	select {
	case pool.ch <- b:
	default:
	}
}

// clear empties the buffer so that all items can be garbage collected.
func (pool *BufferPool) clear() {
	pool.clearOnce.Do(func() {
		ch := pool.ch
		pool.chM.Lock()
		pool.ch = nil
		pool.chM.Unlock()
		close(ch)
		for range ch {
		}
	})
}
