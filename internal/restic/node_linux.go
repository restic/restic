package restic

import (
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	dir, err := fs.Open(filepath.Dir(path))
	if err != nil {
		return errors.WithStack(err)
	}

	times := []unix.Timespec{
		{Sec: utimes[0].Sec, Nsec: utimes[0].Nsec},
		{Sec: utimes[1].Sec, Nsec: utimes[1].Nsec},
	}

	err = unix.UtimesNanoAt(int(dir.Fd()), filepath.Base(path), times, unix.AT_SYMLINK_NOFOLLOW)

	if err != nil {
		// ignore subsequent errors
		_ = dir.Close()
		return errors.Wrap(err, "UtimesNanoAt")
	}

	return dir.Close()
}

func (s statT) atim() syscall.Timespec { return s.Atim }
func (s statT) mtim() syscall.Timespec { return s.Mtim }
func (s statT) ctim() syscall.Timespec { return s.Ctim }
