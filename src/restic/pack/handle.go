package pack

import (
	"fmt"
	"restic/backend"
)

// Handle identifies a blob of a given type.
type Handle struct {
	ID   backend.ID
	Type BlobType
}

func (h Handle) String() string {
	return fmt.Sprintf("<%s/%s>", h.Type, h.ID.Str())
}

// Handles is an ordered list of Handles that implements sort.Interface.
type Handles []Handle

func (h Handles) Len() int {
	return len(h)
}

func (h Handles) Less(i, j int) bool {
	for k, b := range h[i].ID {
		if b == h[j].ID[k] {
			continue
		}

		if b < h[j].ID[k] {
			return true
		}

		return false
	}

	return h[i].Type < h[j].Type
}

func (h Handles) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h Handles) String() string {
	elements := make([]string, 0, len(h))
	for _, e := range h {
		elements = append(elements, e.String())
	}
	return fmt.Sprintf("%v", elements)
}
