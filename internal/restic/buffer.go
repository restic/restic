package restic

import "sync"

// BlobBuffer is a reusable buffer. After the buffer has been used, Release should
// be called so the underlying slice is put back into the pool.
type BlobBuffer struct {
	Data []byte
	pool *BlobBufferPool
}

// Release puts the buffer back into the pool it came from. For buffers created
// with NewBuffer, Release is a no-op.
func (b *BlobBuffer) Release() {
	pool := b.pool
	if pool == nil || cap(b.Data) > pool.defaultSize {
		return
	}

	pool.pool.Put(b)
}

// BlobBufferPool implements a limited set of reusable buffers.
type BlobBufferPool struct {
	pool        sync.Pool
	defaultSize int
}

// NewBlobBufferPool initializes a new buffer pool. New buffers are created
// with defaultSize. Buffers that have grown larger are not put back.
func NewBlobBufferPool(defaultSize int) *BlobBufferPool {
	b := &BlobBufferPool{
		defaultSize: defaultSize,
	}
	b.pool = sync.Pool{New: func() any {
		return &BlobBuffer{
			Data: make([]byte, defaultSize),
			pool: b,
		}
	}}
	return b
}

// Get returns a buffer from the pool. It is guaranteed that the buffer has zero length.
func (pool *BlobBufferPool) Get() *BlobBuffer {
	buf := pool.pool.Get().(*BlobBuffer)
	buf.Data = buf.Data[:0]
	return buf
}

// NewBuffer wraps an existing slice. Release is a no-op.
func NewBuffer(data []byte) *BlobBuffer {
	return &BlobBuffer{Data: data}
}
