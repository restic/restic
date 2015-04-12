package restic

import (
	"crypto/sha256"
	"sync"

	"github.com/restic/restic/chunker"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
)

type poolStats struct {
	m    sync.Mutex
	mget map[string]int
	mput map[string]int
	mmax map[string]int

	new int
	get int
	put int
	max int
}

const maxCiphertextSize = crypto.Extension + chunker.MaxSize

func (s *poolStats) Get(k string) {
	s.m.Lock()
	defer s.m.Unlock()

	s.get += 1
	cur := s.get - s.put
	if cur > s.max {
		s.max = cur
	}

	if k != "" {
		if _, ok := s.mget[k]; !ok {
			s.mget[k] = 0
			s.mput[k] = 0
			s.mmax[k] = 0
		}

		s.mget[k]++

		cur = s.mget[k] - s.mput[k]
		if cur > s.mmax[k] {
			s.mmax[k] = cur
		}
	}
}

func (s *poolStats) Put(k string) {
	s.m.Lock()
	defer s.m.Unlock()

	s.put += 1

	if k != "" {
		s.mput[k]++
	}
}

func newPoolStats() *poolStats {
	return &poolStats{
		mget: make(map[string]int),
		mput: make(map[string]int),
		mmax: make(map[string]int),
	}
}

var (
	chunkPool   = sync.Pool{New: newChunkBuf}
	chunkerPool = sync.Pool{New: newChunker}

	chunkStats   = newPoolStats()
	nodeStats    = newPoolStats()
	chunkerStats = newPoolStats()
)

func newChunkBuf() interface{} {
	chunkStats.m.Lock()
	defer chunkStats.m.Unlock()
	chunkStats.new++

	// create buffer for iv, data and mac
	return make([]byte, maxCiphertextSize)
}

func newChunker() interface{} {
	chunkStats.m.Lock()
	defer chunkStats.m.Unlock()
	chunkStats.new++

	// create a new chunker with a nil reader and null polynomial
	return chunker.New(nil, 0, chunkerBufSize, sha256.New())
}

func GetChunkBuf(s string) []byte {
	chunkStats.Get(s)
	return chunkPool.Get().([]byte)
}

func FreeChunkBuf(s string, buf []byte) {
	chunkStats.Put(s)
	chunkPool.Put(buf)
}

func GetChunker(s string) *chunker.Chunker {
	chunkerStats.Get(s)
	return chunkerPool.Get().(*chunker.Chunker)
}

func FreeChunker(s string, ch *chunker.Chunker) {
	chunkerStats.Put(s)
	chunkerPool.Put(ch)
}

func PoolAlloc() {
	debug.Log("pools.PoolAlloc", "pool stats for chunk: new %d, get %d, put %d, diff %d, max %d\n",
		chunkStats.new, chunkStats.get, chunkStats.put, chunkStats.get-chunkStats.put, chunkStats.max)
	for k, v := range chunkStats.mget {
		debug.Log("pools.PoolAlloc", "pool stats for chunk[%s]: get %d, put %d, diff %d, max %d\n",
			k, v, chunkStats.mput[k], v-chunkStats.mput[k], chunkStats.mmax[k])
	}

	debug.Log("pools.PoolAlloc", "pool stats for node: new %d, get %d, put %d, diff %d, max %d\n",
		nodeStats.new, nodeStats.get, nodeStats.put, nodeStats.get-nodeStats.put, nodeStats.max)
	for k, v := range nodeStats.mget {
		debug.Log("pools.PoolAlloc", "pool stats for node[%s]: get %d, put %d, diff %d, max %d\n", k, v, nodeStats.mput[k], v-nodeStats.mput[k], nodeStats.mmax[k])
	}

	debug.Log("pools.PoolAlloc", "pool stats for chunker: new %d, get %d, put %d, diff %d, max %d\n",
		chunkerStats.new, chunkerStats.get, chunkerStats.put, chunkerStats.get-chunkerStats.put, chunkerStats.max)
	for k, v := range chunkerStats.mget {
		debug.Log("pools.PoolAlloc", "pool stats for chunker[%s]: get %d, put %d, diff %d, max %d\n", k, v, chunkerStats.mput[k], v-chunkerStats.mput[k], chunkerStats.mmax[k])
	}
}
