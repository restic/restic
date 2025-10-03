package fs

import (
	"github.com/restic/restic/internal/data"
	"golang.org/x/sys/unix"
)

// utimesNano is like syscall.UtimesNano, except that it does not follow symlinks.
func utimesNano(path string, atime, mtime int64, _ data.NodeType) error {
	times := []unix.Timespec{
		unix.NsecToTimespec(atime),
		unix.NsecToTimespec(mtime),
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
}
