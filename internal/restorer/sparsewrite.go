//go:build !windows
// +build !windows

package restorer

import (
	"github.com/restic/restic/internal/restic"
)

// WriteAt writes p to f.File at offset. It tries to do a sparse write
// and updates f.size.
func (f *partialFile) WriteAt(p []byte, offset int64) (n int, err error) {
	if !f.sparse {
		return f.File.WriteAt(p, offset)
	}

	n = len(p)

	// Skip the longest all-zero prefix of p.
	// If it's long enough, we can punch a hole in the file.
	skipped := restic.ZeroPrefixLen(p)
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
