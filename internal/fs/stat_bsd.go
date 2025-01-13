//go:build freebsd || darwin || netbsd
// +build freebsd darwin netbsd

package fs

import (
	"os"
	"syscall"
	"time"
)

// extendedStat extracts info into an ExtendedFileInfo for unix based operating systems.
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
	}
}

// RecallOnDataAccess checks windows-specific attributes to determine if a file is a cloud-only placeholder.
func (*ExtendedFileInfo) RecallOnDataAccess() (bool, error) {
	return false, nil
}
