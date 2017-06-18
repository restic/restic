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

// HostsDir is a fuse directory which contains hostnames.
type HostsDir struct {
	inode     uint64
	root      *Root
	snapshots restic.Snapshots
	hosts     map[string]*SnapshotsDir
}

// NewHostsDir returns a new directory containing hostnames, which in
// turn contains snapshots of a single host each.
func NewHostsDir(root *Root, inode uint64, snapshots restic.Snapshots) *HostsDir {
	hosts := make(map[string]restic.Snapshots)
	for _, sn := range snapshots {
		hosts[sn.Hostname] = append(hosts[sn.Hostname], sn)
	}

	debug.Log("create hosts dir with %d snapshots, inode %d", len(hosts), inode)

	d := &HostsDir{
		root:      root,
		inode:     inode,
		snapshots: snapshots,
		hosts:     make(map[string]*SnapshotsDir),
	}

	for hostname, snapshots := range hosts {
		debug.Log("  host %v has %v snapshots", hostname, len(snapshots))
		d.hosts[hostname] = NewSnapshotsDir(root, fs.GenerateDynamicInode(inode, hostname), snapshots)
	}

	return d
}

// Attr returns the attributes for the root node.
func (d *HostsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
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
func (d *HostsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

	for name := range d.hosts {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// Lookup returns a specific entry from the root node.
func (d *HostsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	if dir, ok := d.hosts[name]; ok {
		return dir, nil
	}

	return nil, fuse.ENOENT
}
