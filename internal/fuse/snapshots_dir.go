// +build !openbsd
// +build !windows

package fuse

import (
	"fmt"
	"os"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// SnapshotsDir is a fuse directory which contains snapshots.
type SnapshotsDir struct {
	inode     uint64
	root      *Root
	snapshots restic.Snapshots
	names     map[string]*restic.Snapshot
	latest    string
}

// ensure that *SnapshotsDir implements these interfaces
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})
var _ = fs.NodeReadlinker(&snapshotLink{})

// NewSnapshotsDir returns a new directory containing snapshots.
func NewSnapshotsDir(root *Root, inode uint64, snapshots restic.Snapshots) *SnapshotsDir {
	debug.Log("create snapshots dir with %d snapshots, inode %d", len(snapshots), inode)
	d := &SnapshotsDir{
		root:      root,
		inode:     inode,
		snapshots: snapshots,
		names:     make(map[string]*restic.Snapshot, len(snapshots)),
	}

	// Track latest Snapshot
	var latestTime time.Time
	d.latest = ""

	for _, sn := range snapshots {
		name := sn.Time.Format(time.RFC3339)
		if d.latest == "" || !sn.Time.Before(latestTime) {
			latestTime = sn.Time
			d.latest = name
		}
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
func (d *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
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
func (d *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

	// Latest
	if d.latest != "" {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, "latest"),
			Name:  "latest",
			Type:  fuse.DT_Link,
		})
	}
	return items, nil
}

type snapshotLink struct {
	root     *Root
	inode    uint64
	target   string
	snapshot *restic.Snapshot
}

func newSnapshotLink(ctx context.Context, root *Root, inode uint64, target string, snapshot *restic.Snapshot) (*snapshotLink, error) {
	return &snapshotLink{root: root, inode: inode, target: target, snapshot: snapshot}, nil
}

func (l *snapshotLink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.target, nil
}

func (l *snapshotLink) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = l.inode
	a.Mode = os.ModeSymlink | 0777

	if !l.root.cfg.OwnerIsRoot {
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
	}
	a.Atime = l.snapshot.Time
	a.Ctime = l.snapshot.Time
	a.Mtime = l.snapshot.Time

	a.Nlink = 1

	return nil
}

// Lookup returns a specific entry from the root node.
func (d *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	sn, ok := d.names[name]
	if !ok {
		if name == "latest" && d.latest != "" {
			sn2, ok2 := d.names[d.latest]

			// internal error
			if !ok2 {
				return nil, fuse.ENOENT
			}

			return newSnapshotLink(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), d.latest, sn2)
		}
		return nil, fuse.ENOENT
	}

	return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
}
