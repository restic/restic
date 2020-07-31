package webdav

import (
	"context"
	"io"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/webdav"
)

type fuseDir interface {
	fs.Node
	fs.HandleReadDirAller
	fs.NodeStringLookuper
}

// RepoDir implements a read-only directory
type RepoDir struct {
	name string
	dir  fuseDir
	ctx  context.Context
}

// statically ensure that RepoDir implements webdav.File
var _ webdav.File = &RepoDir{}

func (f *RepoDir) Write(p []byte) (int, error) {
	return 0, webdav.ErrForbidden
}

// Close closes the repo file.
func (f *RepoDir) Close() error {
	debug.Log("Close %v", f.name)
	return nil
}

// Read reads up to len(p) byte from the file.
func (f *RepoDir) Read(p []byte) (int, error) {
	debug.Log("")
	return 0, io.EOF
}

// Seek sets the offset for the next Read or Write to offset, interpreted
// according to whence: SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and SeekEnd means relative
// to the end. Seek returns the new offset relative to the start of the file
// and an error, if any.
func (f *RepoDir) Seek(offset int64, whence int) (int64, error) {
	debug.Log("Seek %v", f.name)
	return 0, webdav.ErrForbidden
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
func (f *RepoDir) Readdir(count int) (entries []os.FileInfo, err error) {
	debug.Log("Readdir %v, count %d", f.name, count)

	dirent, err := f.dir.ReadDirAll(f.ctx)
	entries = make([]os.FileInfo, 0, len(dirent))
	for _, node := range dirent {
		if node.Name == "." || node.Name == ".." {
			continue
		}

		fsNode, err := f.dir.Lookup(f.ctx, node.Name)
		if err != nil {
			return nil, err
		}

		var attr fuse.Attr
		err = fsNode.Attr(f.ctx, &attr)
		if err != nil {
			return nil, err
		}

		entries = append(entries, fileInfoFromAttr(node.Name, attr))
	}
	return entries, nil
}

// Stat returns a FileInfo describing the named file.
func (f *RepoDir) Stat() (os.FileInfo, error) {
	debug.Log("Stat %v", f.name)
	var attr fuse.Attr
	f.dir.Attr(f.ctx, &attr)

	return fileInfoFromAttr(f.name, attr), nil
}
