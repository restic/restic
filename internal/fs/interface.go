package fs

import (
	"io"
	"os"
)

// FS bundles all methods needed for a file system.
type FS interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Stat(name string) (os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
	DeviceID(fi os.FileInfo) (deviceID uint64, err error)
	ExtendedStat(fi os.FileInfo) ExtendedFileInfo

	Join(elem ...string) string
	Separator() string
	Abs(path string) (string, error)
	Clean(path string) string
	VolumeName(path string) string
	IsAbs(path string) bool

	Dir(path string) string
	Base(path string) string
}

// File is an open file on a file system.
type File interface {
	io.Reader
	io.Closer

	Readdirnames(n int) ([]string, error)
	Stat() (os.FileInfo, error)
	Name() string
}
