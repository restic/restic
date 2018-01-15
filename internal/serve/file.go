package serve

import (
	"io"
	"os"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/webdav"
)

// RepoFile implements a read-only directory from a repository.
type RepoFile struct {
	fi   os.FileInfo
	node *restic.Node
}

// statically ensure that RepoFile implements webdav.File
var _ webdav.File = &RepoFile{}

func (f *RepoFile) Write(p []byte) (int, error) {
	return 0, webdav.ErrForbidden
}

// Close closes the repo file.
func (f *RepoFile) Close() error {
	return nil
}

// Read reads up to len(p) byte from the file.
func (f *RepoFile) Read(p []byte) (int, error) {
	// TODO
	return 0, io.EOF
}

// Seek sets the offset for the next Read or Write to offset, interpreted
// according to whence: SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and SeekEnd means relative
// to the end. Seek returns the new offset relative to the start of the file
// and an error, if any.
func (f *RepoFile) Seek(offset int64, whence int) (int64, error) {
	// TODO
	return 0, io.EOF
}

// Readdir reads the contents of the directory associated with file and returns
// a slice of up to n FileInfo values, as would be returned by Lstat, in
// directory order. Subsequent calls on the same file will yield further
// FileInfos.
//
// If n > 0, Readdir returns at most n FileInfo structures. In this case, if
// Readdir returns an empty slice, it will return a non-nil error explaining
// why. At the end of a directory, the error is io.EOF.
//
// If n <= 0, Readdir returns all the FileInfo from the directory in a single
// slice. In this case, if Readdir succeeds (reads all the way to the end of
// the directory), it returns the slice and a nil error. If it encounters an
// error before the end of the directory, Readdir returns the FileInfo read
// until that point and a non-nil error.
func (f *RepoFile) Readdir(count int) ([]os.FileInfo, error) {
	// TODO
	return nil, io.EOF
}

// Stat returns a FileInfo describing the named file.
func (f *RepoFile) Stat() (os.FileInfo, error) {
	return f.fi, nil
}
