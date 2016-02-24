package restic

import (
	"sync"

	"github.com/restic/chunker"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, chunker.MinSize)
	},
}

func getBuf() []byte {
	return bufPool.Get().([]byte)
}

func freeBuf(data []byte) {
	bufPool.Put(data)
}
