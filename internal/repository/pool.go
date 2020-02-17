package repository

import (
	"sync"

	"github.com/restic/chunker"
)

// This pool stores pointers to []byte, for use in Repository.SaveAndEncrypt.
// See the example in the sync package docs for why pointers are used.
var bufPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, chunker.MaxSize/3)
		return &buf
	},
}

func getBuf() *[]byte {
	return bufPool.Get().(*[]byte)
}

func freeBuf(data *[]byte) {
	bufPool.Put(data)
}
