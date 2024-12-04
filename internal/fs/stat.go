package fs

import (
	"os"
	"time"
)

// ExtendedFileInfo is an extended stat_t, filled with attributes that are
// supported by most operating systems. The original FileInfo is embedded.
type ExtendedFileInfo struct {
	Name string
	Mode os.FileMode

	DeviceID  uint64 // ID of device containing the file
	Inode     uint64 // Inode number
	Links     uint64 // Number of hard links
	UID       uint32 // owner user ID
	GID       uint32 // owner group ID
	Device    uint64 // Device ID (if this is a device file)
	BlockSize int64  // block size for filesystem IO
	Blocks    int64  // number of allocated filesystem blocks
	Size      int64  // file size in byte

	AccessTime time.Time // last access time stamp
	ModTime    time.Time // last (content) modification time stamp
	ChangeTime time.Time // last status change time stamp

	// nolint:unused // only used on Windows
	sys any // Value returned by os.FileInfo.Sys()
}

// ExtendedStat returns an ExtendedFileInfo constructed from the os.FileInfo.
func ExtendedStat(fi os.FileInfo) *ExtendedFileInfo {
	if fi == nil {
		panic("os.FileInfo is nil")
	}

	return extendedStat(fi)
}
