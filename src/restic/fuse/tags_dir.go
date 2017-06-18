// +build !openbsd
// +build !windows

package fuse

import (
	"os"
	"restic"
	"restic/debug"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// TagsDir is a fuse directory which contains hostnames.
type TagsDir struct {
	inode     uint64
	root      *Root
	snapshots restic.Snapshots
	tags      map[string]*SnapshotsDir
}

// NewTagsDir returns a new directory containing tags, which in turn contains
// snapshots with this tag set.
func NewTagsDir(root *Root, inode uint64, snapshots restic.Snapshots) *TagsDir {
	tags := make(map[string]restic.Snapshots)
	for _, sn := range snapshots {
		for _, tag := range sn.Tags {
			tags[tag] = append(tags[tag], sn)
		}
	}

	debug.Log("create tags dir with %d snapshots, inode %d", len(tags), inode)

	d := &TagsDir{
		root:      root,
		inode:     inode,
		snapshots: snapshots,
		tags:      make(map[string]*SnapshotsDir),
	}

	for name, snapshots := range tags {
		debug.Log("  tag %v has %v snapshots", name, len(snapshots))
		d.tags[name] = NewSnapshotsDir(root, fs.GenerateDynamicInode(inode, name), snapshots)
	}

	return d
}

// Attr returns the attributes for the root node.
func (d *TagsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555

	if !d.root.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the root node.
func (d *TagsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

	for name := range d.tags {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// Lookup returns a specific entry from the root node.
func (d *TagsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	if dir, ok := d.tags[name]; ok {
		return dir, nil
	}

	return nil, fuse.ENOENT
}
