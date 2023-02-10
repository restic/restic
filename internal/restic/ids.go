package restic

import (
	"encoding/hex"
	"strings"
)

// IDs is a slice of IDs that implements sort.Interface and fmt.Stringer.
type IDs []ID

func (ids IDs) Len() int {
	return len(ids)
}

func (ids IDs) Less(i, j int) bool {
	return string(ids[i][:]) < string(ids[j][:])
}

func (ids IDs) Swap(i, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}

func (ids IDs) String() string {
	var sb strings.Builder
	sb.Grow(1 + (1+2*shortStr)*len(ids))

	buf := make([]byte, 2*shortStr)

	sb.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(' ')
		}
		hex.Encode(buf, id[:shortStr])
		sb.Write(buf)
	}
	sb.WriteByte(']')

	return sb.String()
}
