//go:build windows
// +build windows

package archiver

import (
	"os"

	"github.com/restic/restic/internal/fs"
)

type wrappedFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (fi wrappedFileInfo) Mode() os.FileMode {
	return fi.mode
}

// wrapFileInfo returns a new os.FileInfo with the mode, owner, and group fields changed.
func wrapFileInfo(fi *fs.ExtendedFileInfo) *fs.ExtendedFileInfo {
	// wrap the os.FileInfo and return the modified mode, uid and gid are ignored on Windows
	return fs.ExtendedStat(wrappedFileInfo{
		FileInfo: fi.FileInfo,
		mode:     mockFileInfoMode,
	})
}

// wrapIrregularFileInfo returns a new os.FileInfo with the mode changed to irregular file
func wrapIrregularFileInfo(fi *fs.ExtendedFileInfo) *fs.ExtendedFileInfo {
	return &fs.ExtendedFileInfo{
		FileInfo: wrappedFileInfo{
			FileInfo: fi.FileInfo,
			mode:     (fi.Mode() &^ os.ModeType) | os.ModeIrregular,
		},
	}
}
