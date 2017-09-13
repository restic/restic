package fs

import (
	"path/filepath"
)

// HasPathPrefix returns true if p is a subdir of (or a file within) base. It
// assumes a file system which is case sensitive. For relative paths, false is
// returned.
func HasPathPrefix(base, p string) bool {
	if filepath.VolumeName(base) != filepath.VolumeName(p) {
		return false
	}

	if !filepath.IsAbs(base) || !filepath.IsAbs(p) {
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
