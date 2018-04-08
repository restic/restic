package restorer

// packHeap is a heap of packInfo references
// @see https://golang.org/pkg/container/heap/
// @see https://en.wikipedia.org/wiki/Heap_(data_structure)
type packHeap struct {
	elements []*packInfo

	// returns true if download of any of the files is in progress
	inprogress func(files map[*fileInfo]struct{}) bool
}

func (pq *packHeap) Len() int { return len(pq.elements) }

func (pq *packHeap) Less(a, b int) bool {
	packA, packB := pq.elements[a], pq.elements[b]

	ap := pq.inprogress(packA.files)
	bp := pq.inprogress(packB.files)
	if ap && !bp {
		return true
	}

	if packA.cost < packB.cost {
		return true
	}

	return false
}

func (pq *packHeap) Swap(i, j int) {
	pq.elements[i], pq.elements[j] = pq.elements[j], pq.elements[i]
	pq.elements[i].index = i
	pq.elements[j].index = j
}

func (pq *packHeap) Push(x interface{}) {
	n := len(pq.elements)
	item := x.(*packInfo)
	item.index = n
	pq.elements = append(pq.elements, item)
}

func (pq *packHeap) Pop() interface{} {
	old := pq.elements
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	pq.elements = old[0 : n-1]
	return item
}
