package fuse

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"time"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// DirSnapshots is a fuse directory which contains snapshots.
type DirSnapshots struct {
	inode     uint64
	root      *Root
	snapshots restic.Snapshots
	names     map[string]*restic.Snapshot
}

// ensure that *DirSnapshots implements these interfaces
var _ = fs.HandleReadDirAller(&DirSnapshots{})
var _ = fs.NodeStringLookuper(&DirSnapshots{})

// NewDirSnapshots returns a new directory containing snapshots.
func NewDirSnapshots(root *Root, inode uint64, snapshots restic.Snapshots) *DirSnapshots {
	debug.Log("create snapshots dir with %d snapshots, inode %d", len(snapshots), inode)
	d := &DirSnapshots{
		root:      root,
		inode:     inode,
		snapshots: snapshots,
		names:     make(map[string]*restic.Snapshot, len(snapshots)),
	}

	for _, sn := range snapshots {
		name := sn.Time.Format(time.RFC3339)
		for i := 1; ; i++ {
			if _, ok := d.names[name]; !ok {
				break
			}

			name = fmt.Sprintf("%s-%d", sn.Time.Format(time.RFC3339), i)
		}

		d.names[name] = sn
		debug.Log("  add snapshot %v as dir %v", sn.ID().Str(), name)
	}

	return d
}

// Attr returns the attributes for the root node.
func (d *DirSnapshots) Attr(ctx context.Context, attr *fuse.Attr) error {
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
func (d *DirSnapshots) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

	for name := range d.names {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// Lookup returns a specific entry from the root node.
func (d *DirSnapshots) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	sn, ok := d.names[name]
	if !ok {
		return nil, fuse.ENOENT
	}

	return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
}
