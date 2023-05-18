//go:build windows
// +build windows

package archiver

import (
	"os"
)

type wrappedFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (fi wrappedFileInfo) Mode() os.FileMode {
	return fi.mode
}

// wrapFileInfo returns a new os.FileInfo with the mode, owner, and group fields changed.
func wrapFileInfo(fi os.FileInfo) os.FileInfo {
	// wrap the os.FileInfo and return the modified mode, uid and gid are ignored on Windows
	res := wrappedFileInfo{
		FileInfo: fi,
		mode:     mockFileInfoMode,
	}

	return res
}
