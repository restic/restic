
package webdav

import (
	"os"

	"restic/debug"

	"bazil.org/fuse/fs"

	"golang.org/x/net/webdav"
)

// This is the type for "symlink" nodes we get from the FUSE layer. This is
// necessary because these nodes need to support file-like operations but the
// fuse-layer abstraction only returns a link interface.
type fsLinkType interface {
	fs.NodeReadlinker
	fs.Node
}

// Implements webdav.File for a restic backend
type link struct {
	name string
	linkNode fsLinkType

	target webdav.File
}

func NewLink(name string, linkNode fsLinkType, target webdav.File) *link {
	if linkNode == nil {
		return nil
	}

	return &link{
		name: name,
		linkNode: linkNode,
		target: target,
	}
}

// We don't hold a persistent reference when we're created, so does that mean
// we should do so and clean up the target node here?
func (this *link) Close() error {
	debug.Log(this.name)
	return nil
}

func (this *link) Read(p []byte) (n int, err error) {
	debug.Log(this.name)

	return this.target.Read(p)
}

func (this *link) Write(p []byte) (n int, err error) {
	debug.Log(this.name)

	return this.target.Write(p)
}

func (this *link) Seek(offset int64, whence int) (int64, error) {
	debug.Log(this.name)

	return this.target.Seek(offset, whence)
}

func (this *link) Readdir(count int) ([]os.FileInfo, error) {
	debug.Log(this.name)

	return this.target.Readdir(count)
}

func (this *link) Stat() (os.FileInfo, error) {
	debug.Log(this.name)
	fileInfo := FileInfo{
		Filename: this.name,
	}

	if err := this.linkNode.Attr(ctx, &fileInfo.Attr); err != nil {
		debug.Log("error retrieving attrs for %v : %v", this.name, err)
		return nil, err
	}

	return os.FileInfo(&fileInfo), nil
}