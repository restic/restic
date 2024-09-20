//go:build js
// +build js

package fs

import (
	"os"
	// "syscall"
	// "time"
)

// extendedStat extracts info into an ExtendedFileInfo for unix based operating systems.
func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	return ExtendedFileInfo{}
}
