package fs

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// On Linux, we reimplement Stat and Lstat in terms of unix.Statx,
// which gives access to some interesting information that Stat doesn't provide.

func (fs Local) Stat(name string) (os.FileInfo, error) {
	return statx(name, 0)
}

func (fs Local) Lstat(name string) (os.FileInfo, error) {
	return statx(name, unix.AT_SYMLINK_NOFOLLOW)
}

func statx(name string, flags int) (*statxFileInfo, error) {
	const mask = unix.STATX_BASIC_STATS | unix.STATX_BTIME
	fi := &statxFileInfo{}
	// XXX We could pick FORCE_SYNC or DONT_SYNC instead of SYNC_AS_STAT,
	// to influence the behavior when dealing with remote filesystems.
	err := unix.Statx(unix.AT_FDCWD, name, flags|unix.AT_STATX_SYNC_AS_STAT, mask, &fi.st)
	if err != nil {
		return nil, &os.PathError{Path: name, Op: "statx", Err: err}
	}

	fi.name = filepath.Base(name)
	return fi, nil
}

type statxFileInfo struct {
	name string
	st   unix.Statx_t
}

func (fi *statxFileInfo) Name() string       { return fi.name }
func (fi *statxFileInfo) Size() int64        { return int64(fi.st.Size) }
func (fi *statxFileInfo) ModTime() time.Time { return timeFromStatx(fi.st.Mtime) }
func (fi *statxFileInfo) IsDir() bool        { return fi.Mode().IsDir() }
func (fi *statxFileInfo) Sys() any           { return &fi.st }

// Adapted from os/stat_linux.go in the Go stdlib.
func (fi *statxFileInfo) Mode() os.FileMode {
	mode := os.FileMode(fi.st.Mode & 0o777)

	switch fi.st.Mode & unix.S_IFMT {
	case unix.S_IFBLK:
		mode |= os.ModeDevice
	case unix.S_IFCHR:
		mode |= os.ModeDevice | os.ModeCharDevice
	case unix.S_IFDIR:
		mode |= os.ModeDir
	case unix.S_IFIFO:
		mode |= os.ModeNamedPipe
	case unix.S_IFLNK:
		mode |= os.ModeSymlink
	case unix.S_IFREG:
		// nothing to do
	case unix.S_IFSOCK:
		mode |= os.ModeSocket
	}

	if fi.st.Mode&unix.S_ISGID != 0 {
		mode |= os.ModeSetgid
	}
	if fi.st.Mode&unix.S_ISUID != 0 {
		mode |= os.ModeSetuid
	}
	if fi.st.Mode&unix.S_ISVTX != 0 {
		mode |= os.ModeSticky
	}

	return mode
}

func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	s := fi.Sys().(*unix.Statx_t)

	return ExtendedFileInfo{
		FileInfo:  fi,
		DeviceID:  uint64(s.Rdev_major)<<8 | uint64(s.Rdev_minor),
		Inode:     s.Ino,
		Links:     uint64(s.Nlink),
		UID:       s.Uid,
		GID:       s.Gid,
		Device:    uint64(s.Dev_major)<<8 | uint64(s.Dev_minor),
		BlockSize: int64(s.Blksize),
		Blocks:    int64(s.Blocks),
		Size:      int64(s.Size),

		AccessTime: timeFromStatx(s.Atime),
		ModTime:    timeFromStatx(s.Mtime),
		ChangeTime: timeFromStatx(s.Ctime),
	}
}

func timeFromStatx(ts unix.StatxTimestamp) time.Time {
	return time.Unix(ts.Sec, int64(ts.Nsec))
}
