// +build !openbsd

package fuse

import (
	"encoding/binary"

	"github.com/restic/restic/backend"
)

// inodeFromBackendId returns a unique uint64 from a backend id.
// Endianness has no specific meaning, it is just the simplest way to
// transform a []byte to an uint64
func inodeFromBackendId(id backend.ID) uint64 {
	return binary.BigEndian.Uint64(id[:8])
}
