package crypto

import "sync"

const defaultBufSize = 32 * 1024 // 32KiB

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, defaultBufSize)
	},
}

func getBuffer() []byte {
	return bufPool.Get().([]byte)
}

func freeBuffer(buf []byte) {
	bufPool.Put(buf)
}
