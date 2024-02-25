package rofs

import (
	"io/fs"
	"time"
)

// FileInfo provides information about a file or directory.
type FileInfo struct {
	name    string
	mode    fs.FileMode
	modtime time.Time
	size    int64
}

func (fi FileInfo) Name() string       { return fi.name }
func (fi FileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi FileInfo) ModTime() time.Time { return fi.modtime }
func (fi FileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi FileInfo) Size() int64        { return fi.size }
func (fi FileInfo) Sys() any           { return nil }

var _ fs.FileInfo = FileInfo{}
