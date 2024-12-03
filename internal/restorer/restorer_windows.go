//go:build windows
// +build windows

package restorer

import (
	"strings"

	"github.com/restic/restic/internal/restic"
)

// toComparableFilename returns a filename suitable for equality checks. On Windows, it returns the
// uppercase version of the string. On all other systems, it returns the unmodified filename.
func toComparableFilename(path string) string {
	// apparently NTFS internally uppercases filenames for comparison
	return strings.ToUpper(path)
}

// addFile adds the file to restorer's progress tracker.
// If the node represents an ads file, it only adds the size without counting the ads file.
func (res *Restorer) addFile(node *restic.Node, size uint64) {
	if node.IsMainFile() {
		res.opts.Progress.AddFile(size)
	} else {
		// If this is not the main file, we just want to update the size and not the count.
		res.opts.Progress.AddSize(size)
	}
}

// addSkippedFile adds the skipped file to restorer's progress tracker.
// If the node represents an ads file, it skips the file count.
func (res *Restorer) addSkippedFile(node *restic.Node, location string, size uint64) {
	if node.IsMainFile() {
		res.opts.Progress.AddSkippedFile(location, size)
	}
}
