package fs

import (
	"syscall"

	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/unix"
)

func nodeRestoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	times := []unix.Timespec{
		{Sec: utimes[0].Sec, Nsec: utimes[0].Nsec},
		{Sec: utimes[1].Sec, Nsec: utimes[1].Nsec},
	}

	err := unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
	return errors.Wrap(err, "UtimesNanoAt")
}
