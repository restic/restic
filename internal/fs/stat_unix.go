// +build !windows,!darwin,!freebsd,!netbsd

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/pkg/sftp"
)

// extendedStat extracts info into an ExtendedFileInfo for unix based operating systems.
func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	s, ok := fi.Sys().(*syscall.Stat_t)

	if !ok {
		ftps, ok := fi.Sys().(*sftp.FileStat)
		if !ok {
			panic(fmt.Sprintf("conversion to syscall.Stat_t and sftp.FileStat failed, type is %T", fi.Sys()))
		}

		extFI := ExtendedFileInfo{
			FileInfo: fi,
			UID:      ftps.UID,
			GID:      ftps.GID,
			Size:     int64(ftps.Size),

			AccessTime: time.Unix(int64(ftps.Atime), 0),
			ModTime:    time.Unix(int64(ftps.Mtime), 0),
		}

		extFI.ChangeTime = extFI.ModTime

		return extFI
	}

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
