package webdav

import (
	"io"
	"os"

	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/webdav"
)

// RepoFile implements a link.
// Actually no implementation; this will appear as a zero-sized file.
type RepoLink struct {
	name string
}

// statically ensure that RepoFile implements webdav.File
var _ webdav.File = &RepoLink{}

func (f *RepoLink) Write(p []byte) (int, error) {
	return 0, webdav.ErrForbidden
}

func (f *RepoLink) Close() error {
	debug.Log("Close %v", f.name)
	return nil
}

// Read reads up to len(p) byte from the file.
func (f *RepoLink) Read(p []byte) (int, error) {
	debug.Log("Read %v, count %d", f.name, len(p))
	return 0, io.EOF
}

func (f *RepoLink) Seek(offset int64, whence int) (int64, error) {
	debug.Log("Seek %v, offset: %d", f.name, offset)
	return 0, io.EOF
}

func (f *RepoLink) Readdir(count int) ([]os.FileInfo, error) {
	return nil, io.EOF
}

// Stat returns a FileInfo describing the named file.
func (f *RepoLink) Stat() (os.FileInfo, error) {
	debug.Log("Stat %v", f.name)
	return &virtFileInfo{
		name:  f.name,
		size:  0,
		isdir: false,
	}, nil
}
