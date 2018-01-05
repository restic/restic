package fs

import (
	"path/filepath"
)

// HasPathPrefix returns true if p is a subdir of (or a file within) base. It
// assumes a file system which is case sensitive. If the paths are not of the
// same type (one is relative, the other is absolute), false is returned.
func HasPathPrefix(base, p string) bool {
	if filepath.VolumeName(base) != filepath.VolumeName(p) {
		return false
	}

	// handle case when base and p are not of the same type
	if filepath.IsAbs(base) != filepath.IsAbs(p) {
		return false
	}

	base = filepath.Clean(base)
	p = filepath.Clean(p)

	if base == p {
		return true
	}

	for {
		dir := filepath.Dir(p)

		if base == dir {
			return true
		}

		if p == dir {
			break
		}

		p = dir
	}

	return false
}
