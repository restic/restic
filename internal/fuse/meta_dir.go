// +build darwin freebsd linux

package fuse

import (
	"context"
	"os"

	"github.com/restic/restic/internal/debug"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// ensure that *DirSnapshots implements these interfaces
var _ = fs.HandleReadDirAller(&MetaDir{})
var _ = fs.NodeStringLookuper(&MetaDir{})

// MetaDir is a fuse directory which contains other directories.
type MetaDir struct {
	inode   uint64
	root    *Root
	entries map[string]fs.Node
}

// NewMetaDir returns a new meta dir.
func NewMetaDir(root *Root, inode uint64, entries map[string]fs.Node) *MetaDir {
	debug.Log("new meta dir with %d entries, inode %d", len(entries), inode)

	return &MetaDir{
		root:    root,
		inode:   inode,
		entries: entries,
	}
}

// Attr returns the attributes for the root node.
func (d *MetaDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555
	attr.Uid = d.root.uid
	attr.Gid = d.root.gid

	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the root node.
func (d *MetaDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")
	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.root.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for name := range d.entries {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// Lookup returns a specific entry from the root node.
func (d *MetaDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	if dir, ok := d.entries[name]; ok {
		return dir, nil
	}

	return nil, fuse.ENOENT
}
