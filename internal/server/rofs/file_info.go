package rofs

import (
	"io/fs"
	"time"
)

// fileInfo provides information about a file or directory.
type fileInfo struct {
	name    string
	mode    fs.FileMode
	modtime time.Time
	size    int64
}

func (fi fileInfo) Name() string       { return fi.name }
func (fi fileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi fileInfo) ModTime() time.Time { return fi.modtime }
func (fi fileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi fileInfo) Size() int64        { return fi.size }
func (fi fileInfo) Sys() any           { return nil }

var _ fs.FileInfo = fileInfo{}
