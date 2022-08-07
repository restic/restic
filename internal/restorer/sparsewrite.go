//go:build !windows
// +build !windows

package restorer

import "bytes"

// WriteAt writes p to f.File at offset. It tries to do a sparse write
// and updates f.size.
func (f *partialFile) WriteAt(p []byte, offset int64) (n int, err error) {
	if !f.sparse {
		return f.File.WriteAt(p, offset)
	}

	n = len(p)

	// Skip the longest all-zero prefix of p.
	// If it's long enough, we can punch a hole in the file.
	skipped := zeroPrefixLen(p)
	p = p[skipped:]
	offset += int64(skipped)

	switch {
	case len(p) == 0:
		// All zeros, file already big enough. A previous WriteAt or
		// Truncate will have produced the zeros in f.File.

	default:
		var n2 int
		n2, err = f.File.WriteAt(p, offset)
		n = skipped + n2
	}

	return n, err
}

// zeroPrefixLen returns the length of the longest all-zero prefix of p.
func zeroPrefixLen(p []byte) (n int) {
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
