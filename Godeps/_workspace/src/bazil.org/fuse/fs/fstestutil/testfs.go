package fstestutil

import (
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// SimpleFS is a trivial FS that just implements the Root method.
type SimpleFS struct {
	Node fs.Node
}

var _ = fs.FS(SimpleFS{})

func (f SimpleFS) Root() (fs.Node, error) {
	return f.Node, nil
}

// File can be embedded in a struct to make it look like a file.
type File struct{}

func (f File) Attr(a *fuse.Attr) {
	a.Mode = 0666
}

// Dir can be embedded in a struct to make it look like a directory.
type Dir struct{}

func (f Dir) Attr(a *fuse.Attr) {
	a.Mode = os.ModeDir | 0777
}

// ChildMap is a directory with child nodes looked up from a map.
type ChildMap map[string]fs.Node

var _ = fs.Node(ChildMap{})
var _ = fs.NodeStringLookuper(ChildMap{})

func (f ChildMap) Attr(a *fuse.Attr) {
	a.Mode = os.ModeDir | 0777
}

func (f ChildMap) Lookup(ctx context.Context, name string) (fs.Node, error) {
	child, ok := f[name]
	if !ok {
		return nil, fuse.ENOENT
	}
	return child, nil
}
