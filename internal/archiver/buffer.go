package archiver

// buffer is a reusable buffer. After the buffer has been used, Release should
// be called so the underlying slice is put back into the pool.
type buffer struct {
	Data []byte
	pool *bufferPool
}

// Release puts the buffer back into the pool it came from.
func (b *buffer) Release() {
	pool := b.pool
	if pool == nil || cap(b.Data) > pool.defaultSize {
		return
	}

	select {
	case pool.ch <- b:
	default:
	}
}

// bufferPool implements a limited set of reusable buffers.
type bufferPool struct {
	ch          chan *buffer
	defaultSize int
}

// newBufferPool initializes a new buffer pool. The pool stores at most max
// items. New buffers are created with defaultSize. Buffers that have grown
// larger are not put back.
func newBufferPool(max int, defaultSize int) *bufferPool {
	b := &bufferPool{
		ch:          make(chan *buffer, max),
		defaultSize: defaultSize,
	}
	return b
}

// Get returns a new buffer, either from the pool or newly allocated.
func (pool *bufferPool) Get() *buffer {
	select {
	case buf := <-pool.ch:
		return buf
	default:
	}

	b := &buffer{
		Data: make([]byte, pool.defaultSize),
		pool: pool,
	}

	return b
}
