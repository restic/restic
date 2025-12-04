package rechunker

import (
	"maps"
	"strings"
	"sync"

	"github.com/restic/restic/internal/debug"
)

// global data structure for debug trace
var debugStats = NewStats(true)

type Stats struct {
	d  map[string]int
	mu sync.Mutex
}

func NewStats(enable bool) *Stats {
	if enable {
		return &Stats{
			d: map[string]int{},
		}
	}
	return nil
}

func (n *Stats) Add(k string, v int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.d[k] += v
}

func (n *Stats) AddMap(m map[string]int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for k, v := range m {
		n.d[k] += v
	}
}

func (n *Stats) UpdateMax(k string, v int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.d[k] < v {
		n.d[k] = v
	}
}

func (n *Stats) Dump() (note map[string]int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	note = map[string]int{}
	maps.Copy(note, n.d)

	return note
}

func debugPrintRechunkReport(rc *Rechunker) {
	if debugStats == nil {
		return
	}

	dNote := debugStats.Dump()

	if rc.cfg.CacheSize > 0 {
		debug.Log("List of blobs downloaded more than once:")
		numBlobRedundant := 0
		redundantDownloadCount := 0
		for k := range dNote {
			if strings.HasPrefix(k, "load:") && dNote[k] > 1 {
				debug.Log("%v: Downloaded %d times", k[5:15], dNote[k])
				numBlobRedundant++
				redundantDownloadCount += dNote[k]
			}
		}
		debug.Log("[summary_blobcache] Number of redundantly downloaded blobs is %d, whose overall download count is %d", numBlobRedundant, redundantDownloadCount)
		debug.Log("[summary_blobcache] Peak memory usage by blob cache: %v/%v bytes", dNote["max_cache_usage"], rc.cfg.CacheSize)
		if dNote["total_blob_count"] != dNote["ignored_blob_count"] {
			debug.Log("[summary_blobcache] WARNING: Number of successfully ignored blob at the end: %v/%v", dNote["ignored_blob_count"], dNote["total_blob_count"])
		}
	}
}
