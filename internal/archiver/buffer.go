package archiver

// Buffer is a reusable buffer. After the buffer has been used, Release should
// be called so the underlying slice is put back into the pool.
type Buffer struct {
	Data []byte
	pool *BufferPool
}

// Release puts the buffer back into the pool it came from.
func (b *Buffer) Release() {
	pool := b.pool
	if pool == nil || cap(b.Data) > pool.defaultSize {
		return
	}

	select {
	case pool.ch <- b:
	default:
	}
}

// BufferPool implements a limited set of reusable buffers.
type BufferPool struct {
	ch          chan *Buffer
	defaultSize int
}

// NewBufferPool initializes a new buffer pool. The pool stores at most max
// items. New buffers are created with defaultSize. Buffers that have grown
// larger are not put back.
func NewBufferPool(max int, defaultSize int) *BufferPool {
	b := &BufferPool{
		ch:          make(chan *Buffer, max),
		defaultSize: defaultSize,
	}
	return b
}

// Get returns a new buffer, either from the pool or newly allocated.
func (pool *BufferPool) Get() *Buffer {
	select {
	case buf := <-pool.ch:
		return buf
	default:
	}

	b := &Buffer{
		Data: make([]byte, pool.defaultSize),
		pool: pool,
	}

	return b
}
