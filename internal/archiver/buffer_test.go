package archiver

import (
	"testing"
)

func TestBufferPoolReuse(t *testing.T) {
	success := false
	// retries to avoid flakiness. The test can fail depending on the GC.
	for i := 0; i < 100; i++ {
		// Test that buffers are actually reused from the pool
		pool := newBufferPool(1024)

		// Get a buffer and modify it
		buf1 := pool.Get()
		buf1.Data[0] = 0xFF
		originalAddr := &buf1.Data[0]
		buf1.Release()

		// Get another buffer and check if it's the same underlying slice
		buf2 := pool.Get()
		if &buf2.Data[0] == originalAddr {
			success = true
			break
		}
		buf2.Release()
	}
	if !success {
		t.Error("buffer was not reused from pool")
	}
}

func TestBufferPoolLargeBuffers(t *testing.T) {
	success := false
	// retries to avoid flakiness. The test can fail depending on the GC.
	for i := 0; i < 100; i++ {
		// Test that buffers larger than defaultSize are not returned to pool
		pool := newBufferPool(1024)
		buf := pool.Get()

		// Grow the buffer beyond default size
		buf.Data = append(buf.Data, make([]byte, 2048)...)
		originalCap := cap(buf.Data)

		buf.Release()

		// Get a new buffer - should not be the same slice
		newBuf := pool.Get()
		if cap(newBuf.Data) != originalCap {
			success = true
			break
		}
	}

	if !success {
		t.Error("large buffer was incorrectly returned to pool")
	}
}
