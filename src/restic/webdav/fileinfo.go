package webdav

import (
	"os"
	"time"
	"bazil.org/fuse"
)

// Implements os.FileInfo on top of fuse.Attr
type FileInfo struct {
	// The file basename
	Filename string
	// Underlying fuse.Attr provider of the file
	fuse.Attr
}

func (this FileInfo) Name() string       {
	return this.Filename
}

func (this FileInfo) Size() int64        {
	return int64(this.Attr.Size)
}

func (this FileInfo) Mode() os.FileMode     {
	return this.Attr.Mode
}

func (this FileInfo) ModTime() time.Time {
	return this.Attr.Mtime
}

func (this FileInfo) IsDir() bool        {
	return (this.Attr.Mode & os.ModeDir) != 0
}

func (this FileInfo) Sys() interface{}   {
	return &this.Attr
}