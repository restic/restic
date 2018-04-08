package repository

import (
	"sync"

	"github.com/restic/chunker"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, chunker.MaxSize/3)
	},
}

func getBuf() []byte {
	return bufPool.Get().([]byte)
}

func freeBuf(data []byte) {
	bufPool.Put(data)
}
