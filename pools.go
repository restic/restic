package restic

import (
	"sync"

	"github.com/restic/restic/debug"
)

type poolStats struct {
	m    sync.Mutex
	mget map[string]int
	mput map[string]int
	mmax map[string]int

	get int
	put int
	max int
}

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
	chunkPool = sync.Pool{New: newChunkBuf}
	nodePool  = sync.Pool{New: newNode}

	chunkStats = newPoolStats()
	nodeStats  = newPoolStats()
)

func newChunkBuf() interface{} {
	// create buffer for iv, data and hmac
	return make([]byte, maxCiphertextSize)
}

func newNode() interface{} {
	// create buffer for iv, data and hmac
	return new(Node)
}

func GetChunkBuf(s string) []byte {
	chunkStats.Get(s)
	return chunkPool.Get().([]byte)
}

func FreeChunkBuf(s string, buf []byte) {
	chunkStats.Put(s)
	chunkPool.Put(buf)
}

func GetNode() *Node {
	nodeStats.Get("")
	return nodePool.Get().(*Node)
}

func FreeNode(n *Node) {
	nodeStats.Put("")
	nodePool.Put(n)
}

func PoolAlloc() {
	debug.Log("pools.PoolAlloc", "pool stats for chunk: get %d, put %d, diff %d, max %d\n", chunkStats.get, chunkStats.put, chunkStats.get-chunkStats.put, chunkStats.max)
	for k, v := range chunkStats.mget {
		debug.Log("pools.PoolAlloc", "pool stats for chunk[%s]: get %d, put %d, diff %d, max %d\n", k, v, chunkStats.mput[k], v-chunkStats.mput[k], chunkStats.mmax[k])
	}

	debug.Log("pools.PoolAlloc", "pool stats for node: get %d, put %d, diff %d, max %d\n", nodeStats.get, nodeStats.put, nodeStats.get-nodeStats.put, nodeStats.max)
	for k, v := range nodeStats.mget {
		debug.Log("pools.PoolAlloc", "pool stats for node[%s]: get %d, put %d, diff %d, max %d\n", k, v, nodeStats.mput[k], v-nodeStats.mput[k], nodeStats.mmax[k])
	}
}
