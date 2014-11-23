package khepri

import "sync"

var (
	chunkPool = sync.Pool{New: newChunkBuf}
	nodePool  = sync.Pool{New: newNode}
)

type alloc_stats struct {
	m         sync.Mutex
	alloc_map map[string]int
	free_map  map[string]int
	alloc     int
	free      int
	new       int
	all       int
	max       int
}

var (
	chunk_stats alloc_stats
	node_stats  alloc_stats
)

func init() {
	chunk_stats.alloc_map = make(map[string]int)
	chunk_stats.free_map = make(map[string]int)
}

func newChunkBuf() interface{} {
	chunk_stats.m.Lock()
	chunk_stats.new += 1
	chunk_stats.m.Unlock()

	// create buffer for iv, data and hmac
	return make([]byte, MaxCiphertextSize)
}

func newNode() interface{} {
	node_stats.m.Lock()
	node_stats.new += 1
	node_stats.m.Unlock()

	// create buffer for iv, data and hmac
	return new(Node)
}

func GetChunkBuf(s string) []byte {
	chunk_stats.m.Lock()
	if _, ok := chunk_stats.alloc_map[s]; !ok {
		chunk_stats.alloc_map[s] = 0
	}
	chunk_stats.alloc_map[s] += 1
	chunk_stats.all += 1
	if chunk_stats.all > chunk_stats.max {
		chunk_stats.max = chunk_stats.all
	}
	chunk_stats.m.Unlock()

	return chunkPool.Get().([]byte)
}

func FreeChunkBuf(s string, buf []byte) {
	chunk_stats.m.Lock()
	if _, ok := chunk_stats.free_map[s]; !ok {
		chunk_stats.free_map[s] = 0
	}
	chunk_stats.free_map[s] += 1
	chunk_stats.all -= 1
	chunk_stats.m.Unlock()

	chunkPool.Put(buf)
}

func GetNode() *Node {
	node_stats.m.Lock()
	node_stats.alloc += 1
	node_stats.all += 1
	if node_stats.all > node_stats.max {
		node_stats.max = node_stats.all
	}
	node_stats.m.Unlock()
	return nodePool.Get().(*Node)
}

func FreeNode(n *Node) {
	node_stats.m.Lock()
	node_stats.all -= 1
	node_stats.free += 1
	node_stats.m.Unlock()
	nodePool.Put(n)
}

func PoolAlloc() {
	// fmt.Fprintf(os.Stderr, "alloc max: %d, new: %d\n", chunk_stats.max, chunk_stats.new)
	// for k, v := range chunk_stats.alloc_map {
	// 	fmt.Fprintf(os.Stderr, "alloc[%s] %d, free %d diff: %d\n", k, v, chunk_stats.free_map[k], v-chunk_stats.free_map[k])
	// }

	// fmt.Fprintf(os.Stderr, "nodes alloc max: %d, new: %d\n", node_stats.max, node_stats.new)
	// fmt.Fprintf(os.Stderr, "alloc %d, free %d diff: %d\n", node_stats.alloc, node_stats.free, node_stats.alloc-node_stats.free)
}
