package pack

import (
	"encoding/binary"
	"math/bits"
)

// Computes a padding size using the Padmé algorithm from
// https://lbarman.ch/blog/padme/.
func padmé(size uint) uint {
	if size <= 0 {
		return size
	}

	n := uint64(size)
	log := 64 - bits.LeadingZeros64(n) - 1
	loglog := bits.UintSize - bits.LeadingZeros(uint(log))
	last := log - loglog
	mask := uint64(1)<<last - 1
	padded := uint64(n+mask) &^ mask
	return uint(padded - n)
}

// Returns a skippable zstd frame of the desired size.
// A skippable frame represents the empty blob.
// The size is exact except that sizes >0, <8 are mapped to 8.
func skippableFrame(size uint32) []byte {
	if size == 0 {
		return nil
	}

	const headerSize = 4 + 4
	size = max(size, headerSize)

	// https://www.rfc-editor.org/rfc/rfc8878.pdf#name-skippable-frames
	const magic = 0x184D2A50
	p := make([]byte, size)
	binary.LittleEndian.PutUint32(p[:4], magic)
	binary.LittleEndian.PutUint32(p[4:], size-headerSize)
	return p
}
