package fs

import (
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// Reader is a file system which provides a directory with a single file. When
// this file is opened for reading, the reader is passed through. The file can
// be opened once, all subsequent open calls return syscall.EIO. For Lstat(),
// the provided FileInfo is returned.
type Reader struct {
	Name string
	io.ReadCloser

	// for FileInfo
	Mode    os.FileMode
	ModTime time.Time
	Size    int64

	AllowEmptyFile bool

	open sync.Once
}

// statically ensure that Local implements FS.
var _ FS = &Reader{}

// VolumeName returns leading volume name, for the Reader file system it's
// always the empty string.
func (fs *Reader) VolumeName(_ string) string {
	return ""
}

func (fs *Reader) fi() *ExtendedFileInfo {
	return &ExtendedFileInfo{
		Name:    fs.Name,
		Mode:    fs.Mode,
		ModTime: fs.ModTime,
		Size:    fs.Size,
	}
}

func (fs *Reader) OpenFile(name string, flag int, _ bool) (f File, err error) {
	if flag & ^(O_RDONLY|O_NOFOLLOW) != 0 {
		return nil, pathError("open", name,
			fmt.Errorf("invalid combination of flags 0x%x", flag))
	}

	switch name {
	case fs.Name:
		fs.open.Do(func() {
			f = newReaderFile(fs.ReadCloser, fs.fi(), fs.AllowEmptyFile)
		})

		if f == nil {
			return nil, pathError("open", name, syscall.EIO)
		}

		return f, nil
	case "/", ".":
		f = fakeDir{
			entries: []string{fs.fi().Name},
		}
		return f, nil
	}

	return nil, pathError("open", name, syscall.ENOENT)
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *os.PathError.
func (fs *Reader) Lstat(name string) (*ExtendedFileInfo, error) {
	getDirInfo := func(name string) *ExtendedFileInfo {
		return &ExtendedFileInfo{
			Name:    fs.Base(name),
			Size:    0,
			Mode:    os.ModeDir | 0755,
			ModTime: time.Now(),
		}
	}

	switch name {
	case fs.Name:
		return fs.fi(), nil
	case "/", ".":
		return getDirInfo(name), nil
	}

	dir := fs.Dir(fs.Name)
	for {
		if dir == "/" || dir == "." {
			break
		}
		if name == dir {
			return getDirInfo(name), nil
		}
		dir = fs.Dir(dir)
	}

	return nil, pathError("lstat", name, os.ErrNotExist)
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls Clean on the result; in particular, all
// empty strings are ignored. On Windows, the result is a UNC path if and only
// if the first path element is a UNC path.
func (fs *Reader) Join(elem ...string) string {
	return path.Join(elem...)
}

// Separator returns the OS and FS dependent separator for dirs/subdirs/files.
func (fs *Reader) Separator() string {
	return "/"
}

// IsAbs reports whether the path is absolute. For the Reader, this is always the case.
func (fs *Reader) IsAbs(_ string) bool {
	return true
}

// Abs returns an absolute representation of path. If the path is not absolute
// it will be joined with the current working directory to turn it into an
// absolute path. The absolute path name for a given file is not guaranteed to
// be unique. Abs calls Clean on the result.
//
// For the Reader, all paths are absolute.
func (fs *Reader) Abs(p string) (string, error) {
	return path.Clean(p), nil
}

// Clean returns the cleaned path. For details, see filepath.Clean.
func (fs *Reader) Clean(p string) string {
	return path.Clean(p)
}

// Base returns the last element of p.
func (fs *Reader) Base(p string) string {
	return path.Base(p)
}

// Dir returns p without the last element.
func (fs *Reader) Dir(p string) string {
	return path.Dir(p)
}

func newReaderFile(rd io.ReadCloser, fi *ExtendedFileInfo, allowEmptyFile bool) *readerFile {
	return &readerFile{
		ReadCloser:     rd,
		AllowEmptyFile: allowEmptyFile,
		fakeFile: fakeFile{
			fi:   fi,
			name: fi.Name,
		},
	}
}

type readerFile struct {
	io.ReadCloser
	AllowEmptyFile, bytesRead bool

	fakeFile
}

// ErrFileEmpty is returned inside a *os.PathError by Read() for the file
// opened from the fs provided by Reader when no data could be read and
// AllowEmptyFile is not set.
var ErrFileEmpty = errors.New("no data read")

func (r *readerFile) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.bytesRead = true
	}

	// return an error if we did not read any data
	if err == io.EOF && !r.AllowEmptyFile && !r.bytesRead {
		return n, pathError("read", r.fakeFile.name, ErrFileEmpty)
	}

	return n, err
}

func (r *readerFile) Close() error {
	return r.ReadCloser.Close()
}

// ensure that readerFile implements File
var _ File = &readerFile{}

// fakeFile implements all File methods, but only returns errors for anything
// except Stat()
type fakeFile struct {
	name string
	fi   *ExtendedFileInfo
}

// ensure that fakeFile implements File
var _ File = fakeFile{}

func (f fakeFile) MakeReadable() error {
	return nil
}

func (f fakeFile) Readdirnames(_ int) ([]string, error) {
	return nil, pathError("readdirnames", f.name, os.ErrInvalid)
}

func (f fakeFile) Read(_ []byte) (int, error) {
	return 0, pathError("read", f.name, os.ErrInvalid)
}

func (f fakeFile) ReadAt(_ []byte, _ int64) (n int, err error) {
	return 0, pathError("readAt", f.name, os.ErrInvalid)
}

func (f fakeFile) Close() error {
	return nil
}

func (f fakeFile) Stat() (*ExtendedFileInfo, error) {
	return f.fi, nil
}

func (f fakeFile) ToNode(_ bool) (*restic.Node, error) {
	node := buildBasicNode(f.name, f.fi)

	// fill minimal info with current values for uid, gid
	node.UID = uint32(os.Getuid())
	node.GID = uint32(os.Getgid())
	node.ChangeTime = node.ModTime

	return node, nil
}

// fakeDir implements Readdirnames and Readdir, everything else is delegated to fakeFile.
type fakeDir struct {
	entries []string
	fakeFile
}

func (d fakeDir) Readdirnames(n int) ([]string, error) {
	if n > 0 {
		return nil, pathError("readdirnames", d.name, errors.New("not implemented"))
	}
	return slices.Clone(d.entries), nil
}

func pathError(op, name string, err error) *os.PathError {
	return &os.PathError{Op: op, Path: name, Err: err}
}
