//go:build !windows && !darwin && !freebsd && !netbsd
// +build !windows,!darwin,!freebsd,!netbsd

package fs

import (
	"os"
	"syscall"
	"time"
)

// extendedStat extracts info into an ExtendedFileInfo for unix based operating systems.
func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	s := fi.Sys().(*syscall.Stat_t)

	extFI := ExtendedFileInfo{
		FileInfo:  fi,
		DeviceID:  uint64(s.Dev),
		Inode:     s.Ino,
		Links:     uint64(s.Nlink),
		UID:       s.Uid,
		GID:       s.Gid,
		Device:    uint64(s.Rdev),
		BlockSize: int64(s.Blksize),
		Blocks:    s.Blocks,
		Size:      s.Size,

		AccessTime: time.Unix(s.Atim.Unix()),
		ModTime:    time.Unix(s.Mtim.Unix()),
		ChangeTime: time.Unix(s.Ctim.Unix()),
	}

	return extFI
}
