package restic

import (
	"encoding/hex"
	"fmt"
)

// IDs is an ordered list of IDs that implements sort.Interface.
type IDs []ID

func (ids IDs) Len() int {
	return len(ids)
}

func (ids IDs) Less(i, j int) bool {
	return ids[i].Less(ids[j])
}

func (ids IDs) Swap(i, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}

// Uniq returns list without duplicate IDs. The returned list retains the order
// of the original list so that the order of the first occurrence of each ID
// stays the same.
func (ids IDs) Uniq() (list IDs) {
	seen := NewIDSet()

	for _, id := range ids {
		if seen.Has(id) {
			continue
		}

		list = append(list, id)
		seen.Insert(id)
	}

	return list
}

type shortID ID

func (id shortID) String() string {
	return hex.EncodeToString(id[:shortStr])
}

func (ids IDs) String() string {
	elements := make([]shortID, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, shortID(id))
	}
	return fmt.Sprintf("%v", elements)
}
