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

// wrapIrregularFileInfo returns a new os.FileInfo with the mode changed to irregular file
func wrapIrregularFileInfo(fi os.FileInfo) os.FileInfo {
	return wrappedFileInfo{
		FileInfo: fi,
		mode:     (fi.Mode() &^ os.ModeType) | os.ModeIrregular,
	}
}
