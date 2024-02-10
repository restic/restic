package webdav

import (
	"context"
	"io"
	"os"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/webdav"
)

type fuseFile interface {
	fs.Node
	fs.NodeOpener
}

// RepoFile implements a read-only file
type RepoFile struct {
	name   string
	file   fuseFile
	handle fs.HandleReader
	seek   int64
	size   int64
	ctx    context.Context
}

// statically ensure that RepoFile implements webdav.File
var _ webdav.File = &RepoFile{}

func (f *RepoFile) Write(p []byte) (int, error) {
	return 0, webdav.ErrForbidden
}

// Close closes the repo file.
func (f *RepoFile) Close() error {
	debug.Log("Close %v", f.name)
	return nil
}

// Read reads up to len(p) byte from the file.
func (f *RepoFile) Read(p []byte) (int, error) {
	debug.Log("Read %v, count %d", f.name, len(p))
	var err error

	if f.handle == nil {
		h, err := f.file.Open(f.ctx, nil, nil)
		if err != nil {
			return 0, err
		}
		f.handle = h.(fs.HandleReader)
	}

	maxread := int(f.size - f.seek)
	if len(p) < maxread {
		maxread = len(p)
	}
	if maxread <= 0 {
		return 0, io.EOF
	}
	req := &fuse.ReadRequest{Size: maxread, Offset: f.seek}
	resp := &fuse.ReadResponse{Data: p}
	err = f.handle.Read(f.ctx, req, resp)
	if err != nil {
		return 0, err
	}
	f.seek += int64(len(resp.Data))

	return len(resp.Data), nil
}

// Seek sets the offset for the next Read or Write to offset, interpreted
// according to whence: SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and SeekEnd means relative
// to the end. Seek returns the new offset relative to the start of the file
// and an error, if any.
func (f *RepoFile) Seek(offset int64, whence int) (int64, error) {
	debug.Log("Seek %v, offset: %d", f.name, offset)
	switch whence {
	case os.SEEK_SET:
		f.seek = offset
	case os.SEEK_CUR:
		f.seek += offset
	case os.SEEK_END:
		f.seek = f.size + offset
	}
	if f.seek < 0 || f.seek > f.size {
		return 0, io.EOF
	}

	return f.seek, nil
}

func (f *RepoFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, io.EOF
}

// Stat returns a FileInfo describing the named file.
func (f *RepoFile) Stat() (os.FileInfo, error) {
	debug.Log("Stat %v", f.name)
	var attr fuse.Attr
	f.file.Attr(f.ctx, &attr)

	return fileInfoFromAttr(f.name, attr), nil
}
