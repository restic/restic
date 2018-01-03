// +build freebsd darwin

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// extendedStat extracts info into an ExtendedFileInfo for unix based operating systems.
func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		panic(fmt.Sprintf("conversion to syscall.Stat_t failed, type is %T", fi.Sys()))
	}

	extFI := ExtendedFileInfo{
		FileInfo:  fi,
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
	}

	return extFI
}
