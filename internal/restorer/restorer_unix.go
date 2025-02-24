//go:build !windows
// +build !windows

package restorer

import "github.com/restic/restic/internal/restic"

// toComparableFilename returns a filename suitable for equality checks. On Windows, it returns the
// uppercase version of the string. On all other systems, it returns the unmodified filename.
func toComparableFilename(path string) string {
	return path
}

// addFile adds the file to restorer's progress tracker
func (res *Restorer) addFile(_ *restic.Node, size uint64) {
	res.opts.Progress.AddFile(size)
}

// addSkippedFile adds the skipped file to restorer's progress tracker.
// If the node represents an ads file, it skips the file count.
func (res *Restorer) addSkippedFile(_ *restic.Node, location string, size uint64) {
	res.opts.Progress.AddSkippedFile(location, size)
}
