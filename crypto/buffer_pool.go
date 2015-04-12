package crypto

import "sync"

const defaultBufSize = 2048

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
