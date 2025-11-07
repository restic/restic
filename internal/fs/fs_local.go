package fs

import (
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
)

func init() {
	if err := enableProcessPrivileges(); err != nil {
		debug.Log("error enabling privileges: %v", err)
	}
}

// Local is the local file system. Most methods are just passed on to the stdlib.
type Local struct{}

// statically ensure that Local implements FS.
var _ FS = &Local{}

// VolumeName returns leading volume name. Given "C:\foo\bar" it returns "C:"
// on Windows. Given "\\host\share\foo" it returns "\\host\share". On other
// platforms it returns "".
func (fs Local) VolumeName(path string) string {
	return filepath.VolumeName(path)
}

// OpenFile opens a file or directory for reading.
//
// If metadataOnly is set, an implementation MUST return a File object for
// arbitrary file types including symlinks. The implementation may internally use
// the given file path or a file handle. In particular, an implementation may
// delay actually accessing the underlying filesystem.
//
// Only the O_NOFOLLOW and O_DIRECTORY flags are supported.
func (fs Local) OpenFile(name string, flag int, metadataOnly bool) (File, error) {
	return newLocalFile(name, flag, metadataOnly)
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *PathError.
func (fs Local) Lstat(name string) (*ExtendedFileInfo, error) {
	fi, err := os.Lstat(fixpath(name))
	if err != nil {
		return nil, err
	}
	return extendedStat(fi), nil
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls Clean on the result; in particular, all
// empty strings are ignored. On Windows, the result is a UNC path if and only
// if the first path element is a UNC path.
func (fs Local) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Separator returns the OS and FS dependent separator for dirs/subdirs/files.
func (fs Local) Separator() string {
	return string(filepath.Separator)
}

// IsAbs reports whether the path is absolute.
func (fs Local) IsAbs(path string) bool {
	return filepath.IsAbs(path)
}

// Abs returns an absolute representation of path. If the path is not absolute
// it will be joined with the current working directory to turn it into an
// absolute path. The absolute path name for a given file is not guaranteed to
// be unique. Abs calls Clean on the result.
func (fs Local) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// Clean returns the cleaned path. For details, see filepath.Clean.
func (fs Local) Clean(p string) string {
	return filepath.Clean(p)
}

// Base returns the last element of path.
func (fs Local) Base(path string) string {
	return filepath.Base(path)
}

// Dir returns path without the last element.
func (fs Local) Dir(path string) string {
	return filepath.Dir(path)
}

type localFile struct {
	name string
	flag int
	f    *os.File
	fi   *ExtendedFileInfo
}

// See the File interface for a description of each method
var _ File = &localFile{}

func newLocalFile(name string, flag int, metadataOnly bool) (*localFile, error) {
	var f *os.File
	if !metadataOnly {
		var err error
		f, err = os.OpenFile(fixpath(name), flag, 0)
		if err != nil {
			return nil, err
		}
		_ = setFlags(f)
	}
	return &localFile{
		name: name,
		flag: flag,
		f:    f,
	}, nil
}

func (f *localFile) MakeReadable() error {
	if f.f != nil {
		panic("file is already readable")
	}

	newF, err := newLocalFile(f.name, f.flag, false)
	if err != nil {
		return err
	}
	// replace state and also reset cached FileInfo
	*f = *newF
	return nil
}

func (f *localFile) cacheFI() error {
	if f.fi != nil {
		return nil
	}
	var fi os.FileInfo
	var err error
	if f.f != nil {
		fi, err = f.f.Stat()
	} else if f.flag&O_NOFOLLOW != 0 {
		fi, err = os.Lstat(f.name)
	} else {
		fi, err = os.Stat(f.name)
	}
	if err != nil {
		return err
	}
	f.fi = extendedStat(fi)
	return nil
}

func (f *localFile) Stat() (*ExtendedFileInfo, error) {
	err := f.cacheFI()
	// the call to cacheFI MUST happen before reading from f.fi
	return f.fi, err
}

func (f *localFile) ToNode(ignoreXattrListError bool, warnf func(format string, args ...any)) (*data.Node, error) {
	if err := f.cacheFI(); err != nil {
		return nil, err
	}
	return nodeFromFileInfo(f.name, f.fi, ignoreXattrListError, warnf)
}

func (f *localFile) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

func (f *localFile) Readdirnames(n int) ([]string, error) {
	return f.f.Readdirnames(n)
}

func (f *localFile) Close() error {
	if f.f != nil {
		return f.f.Close()
	}
	return nil
}
