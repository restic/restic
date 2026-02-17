//go:build darwin

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// extendedStat extracts info into an ExtendedFileInfo for macOS.
func extendedStat(fi os.FileInfo) *ExtendedFileInfo {
	s := fi.Sys().(*syscall.Stat_t)

	return &ExtendedFileInfo{
		Name: fi.Name(),
		Mode: fi.Mode(),

		DeviceID:  uint64(s.Dev),
		Inode:     uint64(s.Ino),
		Links:     uint64(s.Nlink),
		UID:       s.Uid,
		GID:       s.Gid,
		Device:    uint64(s.Rdev),
		BlockSize: int64(s.Blksize),
		Blocks:    s.Blocks,
		Size:      s.Size,

		AccessTime: time.Unix(s.Atimespec.Unix()),
		ModTime:    time.Unix(s.Mtimespec.Unix()),
		ChangeTime: time.Unix(s.Ctimespec.Unix()),

		sys: s,
	}
}

// RecallOnDataAccess checks if a file is available locally on the disk or if the file is
// just a dataless files which must be downloaded from a remote server. This is typically used
// in cloud syncing services (e.g. iCloud drive) to prevent downloading files from cloud storage
// until they are accessed.
func (fi *ExtendedFileInfo) RecallOnDataAccess() (bool, error) {
	extAttribute, ok := fi.sys.(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("could not determine file attributes: %s", fi.Name)
	}
	const mask uint32 = unix.SF_DATALESS // 0x40000000
	if extAttribute.Flags&mask == mask {
		return true, nil
	}

	return false, nil
}
