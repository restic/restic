package fs

import (
	"io"
	"os"

	"github.com/restic/restic/internal/restic"
)

// FS bundles all methods needed for a file system.
type FS interface {
	// OpenFile opens a file or directory for reading.
	//
	// If metadataOnly is set, an implementation MUST return a File object for
	// arbitrary file types including symlinks. The implementation may internally use
	// the given file path or a file handle. In particular, an implementation may
	// delay actually accessing the underlying filesystem.
	//
	// Only the O_NOFOLLOW and O_DIRECTORY flags are supported.
	OpenFile(name string, flag int, metadataOnly bool) (File, error)
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

// File is an open file on a file system. When opened as metadataOnly, an
// implementation may opt to perform filesystem operations using the filepath
// instead of actually opening the file.
type File interface {
	// MakeReadable reopens a File that was opened metadataOnly for reading.
	// The method must not be called for files that are opened for reading.
	// If possible, the underlying file should be reopened atomically.
	// MakeReadable must work for files and directories.
	MakeReadable() error

	io.Reader
	io.Closer

	Readdirnames(n int) ([]string, error)
	Stat() (os.FileInfo, error)
	// ToNode returns a restic.Node for the File. The internally used os.FileInfo
	// must be consistent with that returned by Stat(). In particular, the metadata
	// returned by consecutive calls to Stat() and ToNode() must match.
	ToNode(ignoreXattrListError bool) (*restic.Node, error)
}
