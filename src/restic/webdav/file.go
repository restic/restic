
package webdav

import (
	"os"

	"restic/debug"

	"bazil.org/fuse/fs"
	"bazil.org/fuse"
	"io"
)

// This is the type for "file-like" nodes we get from the FUSE layer.
type fsFileType interface {
	fs.HandleReader
	fs.HandleReleaser
	fs.Node
}

// Implements webdav.File for a restic backend
type file struct {
	name string
	fileNode fsFileType

	// Offset into the files content.
	offset int64
}

func NewFile(name string, fileNode fsFileType) *file {
	if fileNode == nil {
		return nil
	}

	return &file{
		name: name,
		fileNode: fileNode,
		offset: 0,
	}
}

// Read-only system never needs to clean up with restic.
func (this *file) Close() error {
	debug.Log(this.name)
	return this.fileNode.Release(ctx, nil)
}

func (this *file) Read(p []byte) (n int, err error) {
	debug.Log(this.name)
	// Construct a read request to the underlying restic fuse.File
	req := &fuse.ReadRequest{
		Dir: false,
		Handle: 0,
		Offset: this.offset,
		Size: len(p),
		Flags: 0,
		LockOwner: 0,
		FileFlags: 0,
	}

	resp := &fuse.ReadResponse{
		Data: p,
	}

	// Stat ourselves to figure out the size below.
	fi, err := this.Stat()
	if err != nil {
		return 0, err
	}

	// Do the read via the fuse wrapper
	if err := this.fileNode.Read(ctx, req, resp); err != nil {
		return 0, err
	}

	// Update the file-offset by the number of read bytes.
	this.offset += int64(len(resp.Data))

	// FIXME: handle if somehow we read past the end of the file?

	// Check for EOF, and if so, return it.
	if this.offset == fi.Size() {
		err = io.EOF
	}

	return len(resp.Data), err
}

func (this *file) Write(p []byte) (n int, err error) {
	debug.Log(this.name)
	return 0, os.ErrInvalid
}

func (this *file) Seek(offset int64, whence int) (int64, error) {
	debug.Log(this.name)
	// Stat ourselves to figure out the size.
	fi, err := this.Stat()
	if err != nil {
		return this.offset, err
	}

	// Can't be negative
	if offset < 0 {
		return this.offset, os.ErrInvalid
	}

	switch whence {
	case io.SeekStart:
		this.offset = offset
		// If seek past end, then seek to end and return EOF
		if this.offset > fi.Size() {
			this.offset = fi.Size()
			return this.offset, io.EOF
		}
		return this.offset, nil
	case io.SeekEnd:
		// If seek past beginning, return an invalid operation error
		if offset > fi.Size() {
			this.offset = 0
			return this.offset, os.ErrInvalid
		}
		this.offset = fi.Size() - offset
		return this.offset, nil
	case io.SeekCurrent:
		if this.offset + offset > fi.Size() {
			this.offset = fi.Size()
			return this.offset, io.EOF
		}
		return this.offset, nil
	default:
		return this.offset, os.ErrInvalid
	}
}

func (this *file) Readdir(count int) ([]os.FileInfo, error) {
	debug.Log(this.name)
	debug.Log("%v", count)
	return []os.FileInfo{}, os.ErrInvalid
}

func (this *file) Stat() (os.FileInfo, error) {
	debug.Log(this.name)
	fileInfo := FileInfo{
		Filename: this.name,
	}

	if err := this.fileNode.Attr(ctx, &fileInfo.Attr); err != nil {
		debug.Log("error retrieving attrs for %v : %v", this.name, err)
		return nil, err
	}

	return os.FileInfo(&fileInfo), nil
}