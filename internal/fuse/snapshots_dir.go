//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"os"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// SnapshotsDir is a actual fuse directory generated from SnapshotsDirStructure
// It uses the saved prefix to filter out the relevant subtrees or entries
// from SnapshotsDirStructure.names and .latest, respectively.
type SnapshotsDir struct {
	root      *Root
	inode     uint64
	dirStruct *SnapshotsDirStructure
	prefix    string
}

// ensure that *SnapshotsDir implements these interfaces
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})

// NewSnapshotsDir returns a new directory structure containing snapshots and "latest" links
func NewSnapshotsDir(root *Root, inode uint64, dirStruct *SnapshotsDirStructure, prefix string) *SnapshotsDir {
	debug.Log("create snapshots dir, inode %d", inode)
	return &SnapshotsDir{
		root:      root,
		inode:     inode,
		dirStruct: dirStruct,
		prefix:    prefix,
	}
}

// Attr returns the attributes for any dir in the snapshots directory structure
func (d *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555
	attr.Uid = d.root.uid
	attr.Gid = d.root.gid

	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the SnapshotsDir.
func (d *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	err := d.dirStruct.updateSnapshots(ctx)
	if err != nil {
		return nil, err
	}

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

	// map to ensure that all names are only listed once
	hasName := make(map[string]struct{})

	for name := range d.dirStruct.names {
		if !strings.HasPrefix(name, d.prefix) {
			continue
		}
		shortname := strings.Split(name[len(d.prefix):], "/")[0]
		if shortname == "" {
			continue
		}
		if _, ok := hasName[shortname]; ok {
			continue
		}
		hasName[shortname] = struct{}{}
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, shortname),
			Name:  shortname,
			Type:  fuse.DT_Dir,
		})
	}

	// Latest
	if _, ok := d.dirStruct.latest[d.prefix]; ok {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, "latest"),
			Name:  "latest",
			Type:  fuse.DT_Link,
		})
	}

	return items, nil
}

// Lookup returns a specific entry from the SnapshotsDir.
func (d *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	err := d.dirStruct.updateSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	fullname := d.prefix + name

	// check if this is already a complete snapshot path
	sn := d.dirStruct.names[fullname]
	if sn != nil {
		return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
	}

	// handle latest case
	if name == "latest" {
		link := d.dirStruct.latest[d.prefix]
		sn := d.dirStruct.names[d.prefix+link]
		if sn != nil {
			return newSnapshotLink(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), link, sn)
		}
	}

	// check if this is a valid subdir
	fullname = fullname + "/"
	for name := range d.dirStruct.names {
		if strings.HasPrefix(name, fullname) {
			return NewSnapshotsDir(d.root, fs.GenerateDynamicInode(d.inode, name), d.dirStruct, fullname), nil
		}
	}

	return nil, fuse.ENOENT
}

// SnapshotLink
type snapshotLink struct {
	root     *Root
	inode    uint64
	target   string
	snapshot *restic.Snapshot
}

var _ = fs.NodeReadlinker(&snapshotLink{})

// newSnapshotLink
func newSnapshotLink(ctx context.Context, root *Root, inode uint64, target string, snapshot *restic.Snapshot) (*snapshotLink, error) {
	return &snapshotLink{root: root, inode: inode, target: target, snapshot: snapshot}, nil
}

// Readlink
func (l *snapshotLink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.target, nil
}

// Attr
func (l *snapshotLink) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = l.inode
	a.Mode = os.ModeSymlink | 0777
	a.Size = uint64(len(l.target))
	a.Blocks = 1 + a.Size/blockSize
	a.Uid = l.root.uid
	a.Gid = l.root.gid
	a.Atime = l.snapshot.Time
	a.Ctime = l.snapshot.Time
	a.Mtime = l.snapshot.Time

	a.Nlink = 1

	return nil
}
