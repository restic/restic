package restic

import (
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"restic/errors"

	"restic/fs"
)

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	dir, err := fs.Open(filepath.Dir(path))
	defer dir.Close()
	if err != nil {
		return errors.Wrap(err, "Open")
	}

	times := []unix.Timespec{
		{Sec: utimes[0].Sec, Nsec: utimes[0].Nsec},
		{Sec: utimes[1].Sec, Nsec: utimes[1].Nsec},
	}

	err = unix.UtimesNanoAt(int(dir.Fd()), filepath.Base(path), times, unix.AT_SYMLINK_NOFOLLOW)

	if err != nil {
		return errors.Wrap(err, "UtimesNanoAt")
	}

	return nil
}

func (s statUnix) atim() syscall.Timespec { return s.Atim }
func (s statUnix) mtim() syscall.Timespec { return s.Mtim }
func (s statUnix) ctim() syscall.Timespec { return s.Ctim }
