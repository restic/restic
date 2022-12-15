package restic

import "bytes"

// ZeroPrefixLen returns the length of the longest all-zero prefix of p.
func ZeroPrefixLen(p []byte) (n int) {
	// First skip 1kB-sized blocks, for speed.
	var zeros [1024]byte

	for len(p) >= len(zeros) && bytes.Equal(p[:len(zeros)], zeros[:]) {
		p = p[len(zeros):]
		n += len(zeros)
	}

	for len(p) > 0 && p[0] == 0 {
		p = p[1:]
		n++
	}

	return n
}
